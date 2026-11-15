package engine

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

// TouchPanDelta returns two-finger center movement in logical pixels.
func (input Input) TouchPanDelta() Vector2 {
	return Vector2{X: input.state.TouchPanX, Y: input.state.TouchPanY}
}

// PinchDelta returns log(previous finger span/current span); spreading is negative.
func (input Input) PinchDelta() float32 { return input.state.Pinch }
