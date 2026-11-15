package ui

import (
	"math"
	"slices"
)

const deviceTouch = "touch"

// Dispatch processes one ordered event synchronously and returns wrapper requests.
func (v *View) Dispatch(event Event) Result {
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

	if v.width == 0 {
		return v.pending
	}

	if v.layoutDirty || v.placementDirty {
		if _, err := v.Frame(v.now); err != nil {
			v.pending.Err = err

			return v.pending
		}
	}

	target := v.eventTarget(event)

	var path [MaxDepth + 1]*Node

	count := 0
	routed := false

	for node := target; node != nil && count < len(path); node = node.parent {
		path[count] = node
		routed = routed || node.preview != nil || node.bubble != nil
		count++
	}

	if routed || len(v.previews) > 0 {
		route := RoutedEvent{Event: event, Target: target}
		v.route(&route, path[:count], event, target)
	} else {
		v.handle(event, target)
	}

	if len(v.modals) > 0 {
		v.pending.Consumed = true
	}

	if event.Kind >= PointerDown && event.Kind <= Wheel && v.overlayContains(event.X, event.Y) {
		v.pending.Consumed = true
	}

	return v.pending
}

func (v *View) handle(event Event, target *Node) {
	if event.Kind == PointerDown || event.Kind == Wheel || event.Kind == Blur {
		v.dismissTooltip()
	}

	switch event.Kind {
	case PointerDown:
		v.pointerDown(event, target)
	case PointerMove:
		v.pointerMove(event)
	case PointerUp:
		v.pointerUp(event)
	case PointerCancel, CaptureLost:
		if v.capture != nil && v.capture.pointer == event.Pointer {
			v.cancelCapture()
			v.pending.Consumed = true
		}
	case Wheel:
		v.wheel(event)
	case KeyDown, KeyUp:
		v.keyboard(event)
	case InsertText, ReplaceText, SetSelection, CompositionStart, CompositionEnd, CompositionCancel:
		v.editEvent(event)
	case Blur:
		v.cancelCapture()
		v.closePopup()
		v.SetFocus(nil)
		v.tooltip.pointerActive = false
		v.hoverAt(-1, -1)
		v.keyPressed = ""
	case Activate:
		if target != nil && target.eligible() && v.inScope(target) {
			v.pulseControlRipple(target)
			v.activate(target)
			v.pending.Consumed = true
		}
	case Increment, Decrement:
		v.accessibleRange(event, target)
	}
}

func (v *View) pointerDown(event Event, target *Node) {
	if target != nil && (!target.eligible() || !v.inScope(target)) {
		return
	}

	if event.Button != 0 || v.capture != nil {
		return
	}

	if v.popup != nil {
		if v.popupRect.Contains(event.X, event.Y) {
			v.popupPointer(event)

			return
		}

		if target != v.popup {
			v.closePopup()
		}
	}

	scroller := v.scrollAt(event.X, event.Y)
	for parent := scroller; parent != nil; parent = parent.parent {
		v.stopScroll(parent)
	}

	if target == nil && scroller == nil {
		if len(v.modals) > 0 {
			return
		}

		v.SetFocus(nil)

		return
	}

	if scroller != nil && v.beginScrollbar(event, scroller) {
		return
	}

	v.focusVisible = false
	if target != nil {
		v.SetFocus(target)
	} else {
		target = scroller
	}

	v.capture = newPressCapture(event, target, scroller)
	v.pressed = target
	target.damage(false)
	v.startControlRipple(target, event.X, event.Y)

	v.pending.Consumed = true
	v.pending.Capture = true

	v.pending.Pointer = event.Pointer
	if target.kind == Slider && event.Device != deviceTouch {
		v.capture.dragging = true
		v.sliderAt(target, event.X)
	}

	if target.kind == Input {
		v.placeCaret(target, event.X, event.Modifiers.Shift)
	}
}
func (v *View) pointerMove(event Event) {
	if v.capture == nil {
		v.tooltipPointer(event.X, event.Y)
		v.hoverAt(event.X, event.Y)

		return
	}

	child := v.capture
	if event.Pointer != child.pointer {
		return
	}

	v.pending.Consumed = true

	if child.scrollbar {
		v.moveScrollbar(child, event)

		return
	}

	if child.popup {
		v.movePopupPointer(child, event)

		return
	}

	dx, dy := event.X-child.positionX, event.Y-child.positionY
	if child.device == deviceTouch && !child.dragging && !child.scrolling && math.Hypot(float64(dx), float64(dy)) >= 8 {
		v.cancelControlRipple(child.node)

		switch {
		case child.scroller != nil && math.Abs(float64(dy)) > math.Abs(float64(dx)):
			child.scrolling = true
			v.pressed = nil

			child.node.damage(false)
		case child.node.kind == Slider:
			child.dragging = true
		default:
			child.dragging = true
			v.pressed = nil

			child.node.damage(false)
		}
	}

	switch {
	case child.scrolling:
		v.scrollBy(child.scroller, child.lastY-event.Y)
	case child.node.kind == Slider && child.dragging:
		v.sliderAt(child.node, event.X)
	case child.node.kind == Input:
		v.placeCaret(child.node, event.X, true)
	case !child.dragging:
		hit := v.hit(event.X, event.Y)
		pressed := v.pressed

		v.pressed = nil
		if hit == child.node {
			v.pressed = child.node
		}

		if pressed != v.pressed {
			v.cancelControlRipple(pressed)
			child.node.damage(false)
		}
	}

	child.lastX, child.lastY = event.X, event.Y
}
func (v *View) pointerUp(event Event) {
	child := v.capture
	if child == nil || child.pointer != event.Pointer || event.Button != 0 {
		return
	}

	v.releaseControlRipple(child.node)

	v.capture = nil
	v.pending.Consumed = true
	v.pending.Release = true
	v.pending.Pointer = event.Pointer
	pressed := v.pressed
	v.pressed = nil

	child.node.damage(false)

	if !child.node.eligible() {
		return
	}

	if child.popup {
		index := int((event.Y - v.popupRect.Y + v.popupOffset) / v.theme.ControlHeight)
		if !child.scrolling && v.popup == child.node && v.popupRect.Contains(event.X, event.Y) && index == child.option &&
			!child.node.options[index].Disabled {
			v.acceptOption()
		}

		return
	}

	if child.node.kind == Slider && !child.scrolling {
		if !child.dragging {
			v.sliderAt(child.node, event.X)
		}

		v.finish(child.node)

		return
	}

	if !child.dragging && !child.scrolling && pressed == child.node && v.hit(event.X, event.Y) == child.node {
		v.activate(child.node)
	}
}
func (v *View) cancelCapture() {
	if v.capture == nil {
		return
	}

	child := v.capture
	v.cancelControlRipple(child.node)
	v.capture = nil
	v.pending.Release = true

	v.pending.Pointer = child.pointer
	if v.pressed != nil {
		v.pressed.damage(false)
	}

	v.pressed = nil

	if child.node.cancel != nil {
		child.node.cancel()
	}
}
func (v *View) activate(node *Node) {
	if !node.eligible() || node.readOnly {
		return
	}

	//nolint:exhaustive // Each controller handles its declared subset; other event/control kinds are ignored or use the default.
	switch node.kind {
	case Checkbox, Switch, ToggleButton:
		node.SetChecked(!node.checked)
		node.changeValue()
		v.finish(node)
	case Button:
		if node.activate != nil {
			node.activate()
		}
	case Select:
		if v.popup == node {
			v.closePopup()
		} else {
			v.popup = node
			v.popupNeedsReveal = true
			node.active = node.selected

			v.nextOption(0)
			v.popupOffset = 0

			node.damage(false)
		}
	}
}
func (v *View) finish(node *Node) {
	if node.view == v && node.commit != nil {
		node.commit(node.changed())
	}
}
func (v *View) adjust(node *Node, value float64) {
	if node.readOnly {
		return
	}

	old := node.value
	if node.SetValue(value) == nil && node.value != old {
		node.changeValue()
	}
}
func (v *View) sliderAt(node *Node, positionX float32) {
	box := node.rect.Inset(14)
	fraction := max(0, min(1, (positionX-box.X)/max(1, box.W)))
	v.adjust(node, node.minimum+float64(fraction)*(node.maximum-node.minimum))
}

func (v *View) hoverAt(positionX, positionY float32) {
	hit := v.hit(positionX, positionY)
	if hit != v.hover {
		v.resumeTooltip()

		if v.hover != nil {
			v.hover.damage(false)
		}

		v.hover = hit
		if hit != nil {
			hit.damage(false)
		}
	}
}

func (v *View) route(route *RoutedEvent, path []*Node, event Event, target *Node) {
	v.previewEvent(route)

	if !route.stopped {
		for _, node := range slices.Backward(path) {
			if node.view == v && node.preview != nil {
				route.Current = node
				node.preview(route)
			}

			if route.stopped {
				break
			}
		}
	}

	if route.prevented {
		v.pending.Consumed = true
	}

	if !route.prevented && (target == nil || target.view == v) {
		v.handle(event, target)
	}

	if !route.stopped {
		for index := range path {
			node := path[index]
			if node.view == v && node.bubble != nil {
				route.Current = node
				node.bubble(route)
			}

			if route.stopped {
				break
			}
		}
	}
}

func (v *View) wheel(event Event) {
	if v.popup != nil && v.popupRect.Contains(event.X, event.Y) {
		v.popupOffset = max(0, min(v.popupExtent(), v.popupOffset+event.DeltaY))
		v.popup.damage(false)
		v.pending.Consumed = true
	} else if node := v.scrollAt(event.X, event.Y); node != nil {
		v.pending.Consumed = v.smoothScrollBy(node, event.DeltaY)
	}
}

func (v *View) accessibleRange(event Event, target *Node) {
	if target != nil && target.eligible() && target.kind == Slider && v.inScope(target) {
		direction := float64(1)
		if event.Kind == Decrement {
			direction = -1
		}

		v.adjust(target, target.value+target.step*direction)
		v.finish(target)
		v.pending.Consumed = true
	}
}

func (v *View) movePopupPointer(child *capture, event Event) {
	if child.device == deviceTouch && abs(event.Y-child.positionY) >= 8 {
		child.scrolling = true
	}

	if child.scrolling {
		v.popupOffset = max(0, min(v.popupExtent(), v.popupOffset+child.lastY-event.Y))
		child.node.damage(false)
	}

	child.lastY = event.Y
}

func (v *View) eventTarget(event Event) *Node {
	target := v.hit(event.X, event.Y)
	if event.Target != 0 {
		target = v.nodes[event.Target]
	}

	if event.Kind >= KeyDown && event.Kind <= CompositionCancel {
		target = v.focus
	}

	if v.capture != nil &&
		(event.Kind == PointerMove || event.Kind == PointerUp || event.Kind == PointerCancel || event.Kind == CaptureLost) {
		target = v.capture.node
	}

	return target
}

func newPressCapture(event Event, target, scroller *Node) *capture {
	return &capture{
		node:       target,
		scroller:   scroller,
		pointer:    event.Pointer,
		positionX:  event.X,
		positionY:  event.Y,
		lastX:      event.X,
		lastY:      event.Y,
		device:     event.Device,
		startValue: target.value,
	}
}
