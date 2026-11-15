package ui

type viewPreview struct {
	callback func(*RoutedEvent)
}

// DispatchPreview processes only View-wide preview handlers. It never lays out
// the View, routes to Nodes, performs default actions or consumes an event based
// on UI focus/bounds. Target and Current are nil. Platform wrappers can use it
// for global shortcuts while another native/HTML control owns keyboard focus.
// Callback mutation, cancellation and nested-dispatch rejection match Dispatch.
func (v *View) DispatchPreview(event Event) Result {
	if v.dispatching {
		return Result{Err: ErrReentrant}
	}

	v.dispatching = true
	defer v.finishDispatch()

	v.pending = Result{}
	if !finite(event.X) || !finite(event.Y) || !finite(event.DeltaX) || !finite(event.DeltaY) {
		v.pending.Err = ErrBounds

		return v.pending
	}

	if len(v.previews) > 0 {
		route := RoutedEvent{Event: event}
		v.previewEvent(&route)
		v.pending.Consumed = route.prevented
	}

	return v.pending
}

func (v *View) finishDispatch() {
	v.dispatching = false
	v.compactPreviews()
}

// OnPreview registers a View-wide input callback, including when no Node is
// focused. It runs before node routing; Current is nil. PreventDefault consumes
// the wrapper event and suppresses its default action. StopPropagation suppresses
// later preview/node callbacks without suppressing the default by itself.
// Registration during dispatch starts on the next event. The returned cancellation
// function is idempotent and immediately suppresses a pending callback.
func (v *View) OnPreview(callback func(*RoutedEvent)) func() {
	if callback == nil {
		return func() {}
	}

	entry := &viewPreview{callback: callback}
	v.previews = append(v.previews, entry)

	return func() {
		if entry.callback == nil {
			return
		}

		entry.callback = nil

		v.previewsDirty = true
		if !v.dispatching {
			v.compactPreviews()
		}
	}
}

func (v *View) previewEvent(event *RoutedEvent) {
	count := len(v.previews)
	for index := range count {
		entry := v.previews[index]
		if entry.callback != nil {
			entry.callback(event)
		}

		if event.stopped {
			return
		}
	}
}

func (v *View) compactPreviews() {
	if !v.previewsDirty {
		return
	}

	kept := v.previews[:0]
	for _, entry := range v.previews {
		if entry.callback != nil {
			kept = append(kept, entry)
		}
	}

	clear(v.previews[len(kept):])
	v.previews, v.previewsDirty = kept, false
}
