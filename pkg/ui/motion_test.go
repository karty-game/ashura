package ui

import (
	"errors"
	"testing"
	"time"
)

func controlMotionFixture() (*View, *Node) {
	view := &View{theme: DarkTheme(), now: time.Second}
	node := &Node{kind: Button, view: view, rect: Rect{X: 10, Y: 20, W: 120, H: 44}, clip: Rect{W: 500, H: 400}}
	view.root = node

	return view, node
}

func controlMotionPaint(view *View, node *Node, fill Color) Color {
	view.animateControl(node, fill, Hex(0xffffff), Hex(0x333333), 8, 1)

	return node.motion.value.fill
}

func controlMotionStep(view *View, now time.Duration) {
	view.now = now
	view.advanceControlMotion(now)
}

func TestControlMotionRetargetsContinuouslyAndSettles(t *testing.T) {
	t.Parallel()

	view, node := controlMotionFixture()

	idle, hovered := Hex(0x202020), Hex(0xc0c0c0)
	if got := controlMotionPaint(view, node, idle); got != idle || len(view.controlMotions) != 0 {
		t.Fatal("first paint must have no entrance animation", got)
	}

	view.hover = node
	controlMotionPaint(view, node, hovered)
	controlMotionStep(view, time.Second+60*time.Millisecond)

	intermediate := node.motion.value.fill
	if intermediate[0] <= idle[0] || intermediate[0] >= hovered[0] {
		t.Fatal("missing intermediate hover color", intermediate)
	}

	view.hover = nil
	if got := controlMotionPaint(view, node, idle); got != intermediate {
		t.Fatal("retarget snapped instead of starting at displayed color", got, intermediate)
	}

	controlMotionStep(view, time.Second+220*time.Millisecond)

	if node.motion.value.fill != idle || len(view.controlMotions) != 0 || view.layoutDirty {
		t.Fatal("transition failed to settle without layout", node.motion.value, len(view.controlMotions))
	}

	node.dirty = false
	view.dirty = view.dirty[:0]
	controlMotionStep(view, 2*time.Second)

	if node.dirty || len(view.dirty) != 0 {
		t.Fatal("settled transition kept damaging its node")
	}
}

func TestInputFocusProgressAndImmediateKeyboardOutline(t *testing.T) {
	t.Parallel()

	view, node := controlMotionFixture()
	node.kind = Input
	controlMotionPaint(view, node, Hex(0x202020))
	view.focus = node
	controlMotionPaint(view, node, Hex(0x303030))
	controlMotionStep(view, time.Second+80*time.Millisecond)

	if focus := view.controlFocus(node); focus <= 0 || focus >= 1 {
		t.Fatal("input focus indicator did not interpolate", focus)
	}

	view.focusVisible = true

	view.animateControl(node, Hex(0x303030), Hex(0xffffff), view.theme.Focus, 8, 2)

	border, width := node.motion.value.border, node.motion.value.width
	if border != view.theme.Focus || width != 2 {
		t.Fatal("keyboard focus outline waited for animation", border, width)
	}

	view.focus = nil
	controlMotionPaint(view, node, Hex(0x202020))
	controlMotionStep(view, time.Second+240*time.Millisecond)

	if view.controlFocus(node) != 0 || len(view.controlMotions) != 0 {
		t.Fatal("blur did not finish focus motion", view.controlFocus(node))
	}
}

func TestControlRippleHoldReleaseAndRapidRepressStayBounded(t *testing.T) {
	t.Parallel()

	view, node := controlMotionFixture()
	view.startControlRipple(node, 20, 25)

	positionX, positionY, initialRadius, opacity, visible := view.controlRipple(node)
	if !visible || positionX != 20 || positionY != 25 || opacity != 1 || initialRadius <= 0 {
		t.Fatal("press ripple lost its local origin", positionX, positionY, initialRadius, opacity, visible)
	}

	controlMotionStep(view, time.Second+320*time.Millisecond)

	positionX, positionY, radius, opacity, visible := view.controlRipple(node)
	if !visible || opacity != 1 || positionX != 20 || positionY != 25 || radius <= initialRadius {
		t.Fatal("ripple did not expand")
	}

	node.dirty = false
	view.dirty = view.dirty[:0]
	controlMotionStep(view, 2*time.Second)

	if node.dirty || len(view.dirty) != 0 {
		t.Fatal("held fully expanded ripple kept uploading paint")
	}

	view.releaseControlRipple(node)
	controlMotionStep(view, 2*time.Second+180*time.Millisecond)

	if _, _, _, _, visible := view.controlRipple(node); visible || len(view.controlMotions) != 0 {
		t.Fatal("released ripple did not retire")
	}

	for range 100 {
		view.startControlRipple(node, 30, 30)
		view.releaseControlRipple(node)
	}

	if len(view.controlMotions) != 1 {
		t.Fatal("rapid presses accumulated animation owners", len(view.controlMotions))
	}

	view.cancelControlRipple(node)
	controlMotionStep(view, 3*time.Second)

	for _, retained := range view.controlMotions[:cap(view.controlMotions)] {
		if retained != nil {
			t.Fatal("retired list retained a node")
		}
	}
}

func TestControlMotionCancellationAndReducedMotion(t *testing.T) {
	t.Parallel()

	for _, scenario := range []string{"hidden", "disabled", "detached", "read only", "reduced motion", "theme disabled"} {
		t.Run(scenario, func(t *testing.T) {
			t.Parallel()

			view, node := controlMotionFixture()
			idle, hover := Hex(0x202020), Hex(0x909090)
			controlMotionPaint(view, node, idle)
			view.hover = node
			controlMotionPaint(view, node, hover)
			view.startControlRipple(node, 20, 25)

			switch scenario {
			case "hidden":
				node.hidden = true
			case "disabled":
				node.disabled = true
			case "detached":
				node.view = nil
			case "read only":
				node.readOnly = true
			case "reduced motion":
				view.SetReducedMotion(true)
			case "theme disabled":
				view.theme.Motion.Disabled = true
			}

			controlMotionStep(view, time.Second+40*time.Millisecond)

			if node.motion.moving || node.motion.ripple.active || len(view.controlMotions) != 0 {
				t.Fatal("inactive control kept interaction motion", scenario, node.motion)
			}

			if scenario == "reduced motion" || scenario == "theme disabled" {
				if got := controlMotionPaint(view, node, hover); got != hover {
					t.Fatal("reduced motion did not snap to state", got)
				}
			}
		})
	}
}

func TestControlMotionClockRewindAndOwnerTransfer(t *testing.T) {
	t.Parallel()

	view, node := controlMotionFixture()
	controlMotionPaint(view, node, Hex(0x202020))
	view.hover = node
	controlMotionPaint(view, node, Hex(0x909090))
	view.startControlRipple(node, 20, 25)
	controlMotionStep(view, time.Second+50*time.Millisecond)

	pose, ripple := node.motion.value, node.motion.ripple

	controlMotionStep(view, -time.Second)

	if node.motion.value != pose || node.motion.ripple.growth != ripple.growth || node.motion.ripple.opacity != ripple.opacity {
		t.Fatal("rewound clock snapped the displayed interaction")
	}

	other, _ := controlMotionFixture()
	node.view = other
	other.startControlRipple(node, 30, 30)
	controlMotionStep(view, 2*time.Second)

	if node.motion.owner != other || !node.motion.listed || len(other.controlMotions) != 1 {
		t.Fatal("old view canceled the new owner's interaction")
	}
}

func TestMotionDurationsAreBounded(t *testing.T) {
	t.Parallel()

	for _, invalid := range []MotionTheme{{HoverDuration: -1}, {RippleFadeDuration: 2*time.Second + 1}} {
		if err := validateMotionTheme(invalid); !errors.Is(err, ErrBounds) {
			t.Fatal("invalid duration was admitted", invalid, err)
		}
	}

	if err := validateMotionTheme(defaultMotionTheme()); err != nil {
		t.Fatal(err)
	}
}
