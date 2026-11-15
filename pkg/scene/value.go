package scene

import "errors"

// ErrReentrant rejects a write back into a property while it is notifying listeners.
// The rejected write leaves its value unchanged; callers must handle the error.
var ErrReentrant = errors.New("reentrant scene property mutation")

type listener[T comparable] struct {
	callback func(T)
	active   bool
}

// Value is explicit observable component data, independent of mesh residency.
// Notifications are synchronous, ordered by subscription, with equal writes suppressed.
// Subscribing during notification takes effect on the next mutation. Unsubscribing
// during notification immediately prevents any later call to that listener.
type Value[T comparable] struct {
	value     T
	listeners []*listener[T]
	notifying bool
	owner     *Root
}

// NewValue sets a component's initial value without emitting a notification.
func NewValue[T comparable](initial T) *Value[T] { return &Value[T]{value: initial} }

// Get reads current component state.
func (value *Value[T]) Get() T { return value.value }

// Set commits a new value and notifies subscribers on the caller's goroutine.
func (value *Value[T]) Set(next T) error {
	if value.owner != nil && value.owner.Closed() {
		return ErrClosed
	}

	if value.value == next {
		return nil
	}

	if value.notifying {
		return ErrReentrant
	}

	value.value = next

	value.notifying = true
	defer func() {
		value.notifying = false
		value.compact()
	}()

	count := len(value.listeners)
	for index := range count {
		entry := value.listeners[index]
		if entry.active {
			entry.callback(next)
		}
	}

	return nil
}

// Subscribe observes subsequent changes and returns an idempotent unsubscribe.
// Apply Get explicitly when a consumer needs an initial presentation value.
func (value *Value[T]) Subscribe(callback func(T)) func() {
	if callback == nil || value.owner != nil && value.owner.Closed() {
		return func() {}
	}

	entry := &listener[T]{callback: callback, active: true}
	value.listeners = append(value.listeners, entry)

	return func() {
		entry.active = false
		entry.callback = nil

		if !value.notifying {
			value.compact()
		}
	}
}

func (value *Value[T]) compact() {
	kept := value.listeners[:0]
	for _, entry := range value.listeners {
		if entry.active {
			kept = append(kept, entry)
		}
	}

	clear(value.listeners[len(kept):])
	value.listeners = kept
}
