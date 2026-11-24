package engine

// LogicalViewportSize returns the viewport size in the same logical pixels as MousePosition.
func (input Input) LogicalViewportSize() (width, height float32) {
	return input.state.LogicalWidth, input.state.LogicalHeight
}

// MouseButtonDown reports whether a pointer button is held.
func (input Input) MouseButtonDown(button MouseButton) bool {
	return int(button) < len(input.state.MouseButtons) && input.state.MouseButtons[button]
}

// WheelDelta returns normalized wheel steps; positive Y scrolls away (zoom out).
// One step is 100 browser pixels, three lines, or one native wheel tick.
func (input Input) WheelDelta() Vector2 {
	return Vector2{X: input.state.WheelX, Y: input.state.WheelY}
}

// TouchDelta returns one-finger dragging since the previous frame in logical pixels.
func (input Input) TouchDelta() Vector2 {
	return Vector2{X: input.state.TouchDX, Y: input.state.TouchDY}
}

// TouchPosition returns a single finger's position in logical viewport pixels.
// The position is available while held and for a tap completed between frames.
// After a multi-touch gesture, all fingers must lift before a new press.
func (input Input) TouchPosition() (position Vector2, active bool) {
	touch := input.state.Touch

	return Vector2{X: touch.X, Y: touch.Y}, touch.Down || touch.Pressed
}

// TouchPanDelta returns two-finger center movement in logical pixels.
func (input Input) TouchPanDelta() Vector2 {
	return Vector2{X: input.state.TouchPanX, Y: input.state.TouchPanY}
}

// PinchDelta returns log(previous finger span/current span); spreading is negative.
func (input Input) PinchDelta() float32 { return input.state.Pinch }
