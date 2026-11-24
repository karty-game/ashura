package platform_test

import (
	"math"
	"testing"

	"github.com/karty-game/ashura/internal/platform"
)

func TestTouchPointerTapAndGestureHandoff(t *testing.T) {
	t.Parallel()

	var tracker platform.TouchTracker

	var input platform.InputState

	one := []platform.TouchPoint{{ID: 1, X: 30, Y: 40}}
	two := []platform.TouchPoint{one[0], {ID: 2, X: 60, Y: 40}}
	tracker.Update(&input, one)

	if tracker.Pointer() != (platform.TouchPointer{X: 30, Y: 40, Down: true}) {
		t.Fatal("held touch was unavailable across frames")
	}

	tracker.Update(&input, nil)

	if input.Touch != (platform.TouchPointer{X: 30, Y: 40, Pressed: true}) || tracker.Pointer().Down {
		t.Fatal("short tap was lost or remained held")
	}

	tracker.Update(&input, one)
	tracker.Update(&input, two)
	tracker.Update(&input, one)

	if input.Touch.Down || input.Touch.Pressed || tracker.Pointer().Down {
		t.Fatal("lifting one finger after a pinch started a paint stroke")
	}

	tracker.Update(&input, nil)
	tracker.Update(&input, one)
	tracker.BlockPointer(&input)
	tracker.Update(&input, one)

	if input.Touch.Down || input.Touch.Pressed {
		t.Fatal("blocked pointer resumed before release")
	}

	tracker.Update(&input, nil)
	tracker.Update(&input, one)

	if !input.Touch.Down || !input.Touch.Pressed {
		t.Fatal("fresh contact failed to start a touch press")
	}
}

func TestTouchDragAndContactTransitions(t *testing.T) {
	t.Parallel()

	var (
		tracker platform.TouchTracker
		input   platform.InputState
	)
	tracker.Update(&input, []platform.TouchPoint{{ID: 1, X: 10, Y: 20}})
	tracker.Update(&input, []platform.TouchPoint{{ID: 1, X: 25, Y: 12}})
	tracker.Update(&input, []platform.TouchPoint{{ID: 1, X: 30, Y: 14}})

	if input.TouchDX != 20 || input.TouchDY != -6 || input.MouseDX != 0 {
		t.Fatalf("one-finger drag did not accumulate independently of mouse: %+v", input)
	}

	input = platform.InputState{}
	tracker.Update(&input, []platform.TouchPoint{{ID: 1, X: 30, Y: 14}, {ID: 2, X: 50, Y: 14}})
	tracker.Update(&input, []platform.TouchPoint{{ID: 1, X: 30, Y: 14}})

	if input.TouchDX != 0 || input.TouchPanX != 0 || input.Pinch != 0 {
		t.Fatal("adding/removing a contact moved the camera")
	}

	tracker.Update(&input, []platform.TouchPoint{{ID: 1, X: 32, Y: 14}})

	if input.TouchDX != 2 {
		t.Fatal("drag did not resume after the contact transition")
	}

	tracker.Reset()

	input = platform.InputState{}
	tracker.Update(&input, []platform.TouchPoint{{ID: 1, X: 900, Y: 900}})

	if input.TouchDX != 0 {
		t.Fatal("canceled gesture resumed with a jump")
	}
}

func TestTwoFingerPanAndPinch(t *testing.T) {
	t.Parallel()

	var (
		tracker platform.TouchTracker
		input   platform.InputState
	)
	tracker.Update(&input, []platform.TouchPoint{{ID: 1}, {ID: 2, X: 10}})
	// Reversing contact order must not reset a continuing gesture.
	tracker.Update(&input, []platform.TouchPoint{{ID: 2, X: 30, Y: 4}, {ID: 1, X: 10, Y: 4}})

	const tolerance = .00001
	if input.TouchPanX != 15 || input.TouchPanY != 4 || math.Abs(float64(input.Pinch)+math.Ln2) > tolerance {
		t.Fatalf("pan/pinch mismatch: %+v", input)
	}

	if input.TouchDX != 0 || input.TouchDY != 0 {
		t.Fatal("two-finger gesture leaked into one-finger look")
	}

	input = platform.InputState{}
	tracker.Update(&input, []platform.TouchPoint{{ID: 1}, {ID: 2}, {ID: 3}})
	tracker.Update(&input, []platform.TouchPoint{{ID: 1, X: 50}, {ID: 2, X: 100}})

	if input.TouchPanX != 0 || input.Pinch != 0 {
		t.Fatal("unsupported contact count did not rebase")
	}
}

func TestCoincidentContactsDoNotProduceInvalidPinch(t *testing.T) {
	t.Parallel()

	var (
		tracker platform.TouchTracker
		input   platform.InputState
	)
	tracker.Update(&input, []platform.TouchPoint{{ID: 1}, {ID: 2}})
	tracker.Update(&input, []platform.TouchPoint{{ID: 1}, {ID: 2, X: 10}})
	tracker.Update(&input, []platform.TouchPoint{{ID: 1}, {ID: 2}})

	if math.IsNaN(float64(input.Pinch)) || math.IsInf(float64(input.Pinch), 0) {
		t.Fatal("invalid pinch from zero span")
	}
}
