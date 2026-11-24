//go:build linux

package native

import (
	"fmt"

	"github.com/Zyko0/go-sdl3/sdl"
	"github.com/karty-game/ashura/internal/platform"
)

// More than two contacts disable gestures. Fixed slots keep contacts stable
// through cancellation/reordering without allocating maps during event polling.
const maximumTouchContacts = 16

type nativeFinger struct {
	device sdl.TouchID
	finger sdl.FingerID
	x, y   float32
	active bool
}

type nativeInput struct {
	touch            platform.TouchTracker
	fingers          [maximumTouchContacts]nativeFinger
	width, height    float32
	overflowContacts int
}

func (driver *Driver) readInput() (platform.InputState, bool, error) {
	var input platform.InputState

	width, height, err := driver.window.Size()
	if err != nil {
		return input, false, fmt.Errorf("read SDL logical window size: %w", err)
	}

	input.LogicalHeight = float32(height)

	input.LogicalWidth = float32(width)
	if driver.uiBridge != nil {
		driver.uiBridge.Resize(float32(width), float32(height))
	}

	state := &driver.navigation
	if state.width != float32(width) || state.height != float32(height) {
		state.width, state.height = float32(width), float32(height)
		state.touch.Reset()
		state.touch.BlockPointer(&input)
		state.publishTouches(&input)
	}

	input.Touch = state.touch.Pointer()

	if !state.pumpEvents(&input, driver) {
		return input, false, nil
	}

	readHeldInput(&input)

	if driver.uiBridge != nil {
		driver.uiBridge.SyncInput()
	}

	return input, true, nil
}

func (state *nativeInput) pumpEvents(input *platform.InputState, driver *Driver) bool {
	var event sdl.Event
	for sdl.PollEvent(&event) {
		consumed := driver.uiBridge != nil && driver.uiBridge.ReadEvent(&event)
		//nolint:exhaustive // Touch releases still reach the tracker when handled by UI.
		switch event.Type {
		case sdl.EVENT_FINGER_DOWN, sdl.EVENT_FINGER_MOTION, sdl.EVENT_FINGER_UP, sdl.EVENT_FINGER_CANCELED:
			if consumed || event.Type == sdl.EVENT_FINGER_CANCELED {
				state.touch.Reset()
				state.touch.BlockPointer(input)
			}

			state.touchEvent(input, event.TouchFingerEvent())

			continue
		default:
		}

		if consumed {
			continue
		}
		//nolint:exhaustive // Other SDL event families are unrelated to navigation.
		switch event.Type {
		case sdl.EVENT_QUIT, sdl.EVENT_WINDOW_CLOSE_REQUESTED:
			return false
		case sdl.EVENT_MOUSE_MOTION:
			motion := event.MouseMotionEvent()
			for index := range input.DragDX {
				if motion.State&(sdl.MouseButtonFlags(1)<<index) != 0 {
					input.DragDX[index] += motion.Xrel
					input.DragDY[index] += motion.Yrel
				}
			}
		case sdl.EVENT_MOUSE_WHEEL:
			wheel := event.MouseWheelEvent()

			direction := float32(1)
			if wheel.Direction == sdl.MOUSEWHEEL_FLIPPED {
				direction = -1
			}

			input.WheelX += wheel.X * direction
			// SDL wheel-up is positive; normalized positive Y means zoom away/out.
			input.WheelY -= wheel.Y * direction
		case sdl.EVENT_WINDOW_FOCUS_LOST:
			state.touch.Reset()
			clear(state.fingers[:])
			state.overflowContacts = 0

			clear(input.DragDX[:])
			clear(input.DragDY[:])
			input.TouchDX, input.TouchDY = 0, 0
			input.TouchPanX, input.TouchPanY, input.Pinch = 0, 0, 0
			input.Touch = platform.TouchPointer{}
		}
	}

	return true
}

func (state *nativeInput) touchEvent(input *platform.InputState, event *sdl.TouchFingerEvent) {
	slot, empty := -1, -1

	for index, contact := range state.fingers {
		if contact.active && contact.device == event.TouchID && contact.finger == event.FingerID {
			slot = index

			break
		}

		if !contact.active && empty < 0 {
			empty = index
		}
	}

	if slot < 0 && event.Type == sdl.EVENT_FINGER_DOWN {
		if empty < 0 {
			state.overflowContacts++
			state.touch.Reset()
			state.touch.BlockPointer(input)

			return
		}

		slot = empty
	}

	if slot < 0 {
		if event.Type == sdl.EVENT_FINGER_UP || event.Type == sdl.EVENT_FINGER_CANCELED {
			if state.overflowContacts > 0 {
				state.overflowContacts--
			}

			state.publishTouches(input)
		}

		return
	}

	contact := &state.fingers[slot]
	if event.Type == sdl.EVENT_FINGER_UP || event.Type == sdl.EVENT_FINGER_CANCELED {
		*contact = nativeFinger{}
	} else {
		*contact = nativeFinger{device: event.TouchID, finger: event.FingerID, x: event.X, y: event.Y, active: true}
	}

	state.publishTouches(input)
}

func (state *nativeInput) publishTouches(input *platform.InputState) {
	if state.overflowContacts > 0 {
		state.touch.BlockPointer(input)

		return
	}

	var points [maximumTouchContacts]platform.TouchPoint

	count := 0

	for index, contact := range state.fingers {
		if !contact.active {
			continue
		}

		points[count] = platform.TouchPoint{ID: int64(index + 1), X: contact.x * state.width, Y: contact.y * state.height}
		count++
	}

	state.touch.Update(input, points[:count])
}

func readHeldInput(input *platform.InputState) {
	keyboard := sdl.GetKeyboardState()

	keys := [...]struct {
		scancode sdl.Scancode
		code     uint8
	}{
		{sdl.SCANCODE_1, platform.KeyCode1},
		{sdl.SCANCODE_2, platform.KeyCode2},
		{sdl.SCANCODE_3, platform.KeyCode3},
		{sdl.SCANCODE_A, platform.KeyCodeA},
		{sdl.SCANCODE_B, platform.KeyCodeB},
		{sdl.SCANCODE_H, platform.KeyCodeH},
		{sdl.SCANCODE_L, platform.KeyCodeL},
		{sdl.SCANCODE_R, platform.KeyCodeR},
		{sdl.SCANCODE_D, platform.KeyCodeD},
		{sdl.SCANCODE_N, platform.KeyCodeN},
		{sdl.SCANCODE_O, platform.KeyCodeO},
		{sdl.SCANCODE_S, platform.KeyCodeS},
		{sdl.SCANCODE_T, platform.KeyCodeT},
		{sdl.SCANCODE_W, platform.KeyCodeW},
		{sdl.SCANCODE_SPACE, platform.KeyCodeSpace},
		{sdl.SCANCODE_LSHIFT, platform.KeyCodeShift},
		{sdl.SCANCODE_RSHIFT, platform.KeyCodeShift},
	}
	for _, key := range keys {
		index := int(key.scancode)
		if index < len(keyboard) && keyboard[index] {
			input.Keys[key.code] = true
		}
	}

	buttons, x, y := sdl.GetMouseState()
	input.MouseButtons[0] = buttons&sdl.ButtonMask(sdl.BUTTON_LEFT) != 0
	input.MouseButtons[1] = buttons&sdl.ButtonMask(sdl.BUTTON_MIDDLE) != 0
	input.MouseButtons[2] = buttons&sdl.ButtonMask(sdl.BUTTON_RIGHT) != 0
	input.MouseX, input.MouseY = x, y
	_, input.MouseDX, input.MouseDY = sdl.GetRelativeMouseState()
}
