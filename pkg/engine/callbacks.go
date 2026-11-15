package engine

import (
	"fmt"
	"time"
)

// callbackList retains only live registrations between dispatches. Removal
// during dispatch is immediate for callers and compacted once at the boundary.
type callbackList[T any] struct {
	entries []*callbackEntry[T]
	running bool
	dirty   bool
}

type callbackEntry[T any] struct {
	callback T
	index    int
	active   bool
}

func (list *callbackList[T]) add(callback T) func() {
	entry := &callbackEntry[T]{callback: callback, index: len(list.entries), active: true}
	list.entries = append(list.entries, entry)

	return func() {
		if !entry.active {
			return
		}

		var empty T

		entry.callback, entry.active = empty, false

		if list.running {
			list.dirty = true

			return
		}

		index := entry.index
		copy(list.entries[index:], list.entries[index+1:])
		list.entries[len(list.entries)-1] = nil
		list.entries = list.entries[:len(list.entries)-1]

		for index := index; index < len(list.entries); index++ {
			list.entries[index].index = index
		}
	}
}

func (list *callbackList[T]) begin() []*callbackEntry[T] {
	list.running = true

	return list.entries
}

func (list *callbackList[T]) end() {
	list.running = false
	if !list.dirty {
		return
	}

	kept := list.entries[:0]
	for _, entry := range list.entries {
		if entry.active {
			entry.index = len(kept)
			kept = append(kept, entry)
		}
	}

	clear(list.entries[len(kept):])

	list.entries, list.dirty = kept, false
	if len(list.entries) == 0 {
		list.entries = nil
	}
}

func (list *callbackList[T]) clear() {
	var empty T
	for _, entry := range list.entries {
		entry.callback, entry.active = empty, false
	}

	if list.running {
		list.dirty = true
	} else {
		list.entries = nil
	}
}

// OnUpdate registers an update and returns an idempotent cancellation function.
// Updates run in registration order before compute and rendering. Registrations
// made during updates start on the next update; cancellation takes effect
// immediately. Registration and cancellation belong to the App loop goroutine.
// Nil callbacks and closed Apps return a harmless cancellation function.
func (app *App) OnUpdate(update func(time.Duration)) func() {
	if update == nil {
		return func() {}
	}

	return app.OnUpdateError(func(delta time.Duration) error {
		update(delta)

		return nil
	})
}

// OnUpdateError registers a fallible update with the same ordering and lifetime
// as OnUpdate. An error stops the frame and is returned by Run, preserving
// errors.Is/errors.As. Handle recoverable gameplay errors inside the callback.
func (app *App) OnUpdateError(update func(time.Duration) error) func() {
	if app == nil || app.closed || update == nil {
		return func() {}
	}

	return app.updates.add(update)
}

func (app *App) runUpdates(delta time.Duration) error {
	entries := app.updates.begin()
	defer app.updates.end()

	for _, entry := range entries {
		if !entry.active {
			continue
		}

		if err := entry.callback(delta); err != nil {
			return fmt.Errorf("update callback: %w", err)
		}

		if app.closed {
			return errAppClosed
		}
	}

	return nil
}
