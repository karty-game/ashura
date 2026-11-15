package ui_test

import (
	"errors"
	"math"
	"strings"
	"testing"

	"github.com/karty-game/karty-webgpu/pkg/ui"
	"github.com/karty-game/karty-webgpu/pkg/ui/fonts/defaultfont"
)

func TestRemovingModalEndsItsFocusScope(t *testing.T) {
	t.Parallel()

	opener := ui.NewButton("Open", nil)

	inside := ui.NewInput("Note", "retained", "", nil)

	dialog := ui.Column(inside)

	dialog.SetHidden(true)

	view := surface(t, ui.Column(opener, dialog))

	view.SetFocus(opener)

	if err := view.PushModal(dialog); err != nil {
		t.Fatal(err)
	}

	paint(t, view)

	dialog.Remove()

	paint(t, view)

	if view.CapturesKeyboard() || view.CapturesPointer() || view.Focused() != opener {
		t.Fatal("removed modal kept its scope or lost the restore target")
	}

	for _, semantic := range view.Semantics() {
		if semantic.ID == inside.ID() || semantic.ID == dialog.ID() {
			t.Fatal("removed modal remained in painted/semantic order")
		}
	}
}

func TestPreviewDisablePreventsSliderDefault(t *testing.T) {
	t.Parallel()

	changes := 0

	slider := ui.NewSlider("Range", 0, 100, 1, 20, func(ui.Change) { changes++ })

	view := surface(t, ui.Column(slider))

	slider.OnPreview(func(event *ui.RoutedEvent) {
		if event.Event.Kind == ui.PointerDown {
			slider.SetDisabled(true)
		}
	})

	positionX, positionY := point(slider)

	result := view.Dispatch(ui.Event{Kind: ui.PointerDown, Pointer: 1, Device: "mouse", X: positionX, Y: positionY})

	if slider.Value() != 20 || changes != 0 || result.Capture || view.CapturesPointer() {
		t.Fatal("disabled preview target performed its default", slider.Value(), changes, result)
	}
}

func TestEditingNavigationBetweenFramesUsesCurrentText(t *testing.T) {
	t.Parallel()

	input := ui.NewInput("Name", "abc", "", nil)

	view := surface(t, ui.Column(input))

	view.SetFocus(input)

	view.Dispatch(ui.Event{Kind: ui.InsertText, Text: "d"})

	key(view, "ArrowLeft")

	state := view.Editing()

	if state.Cursor != 3 || state.Anchor != 3 {
		t.Fatal("caret used old shaped text between events", state.Cursor, state.Anchor)
	}

	key(view, "Backspace")

	if input.Text() != "abd" {
		t.Fatal("navigation/deletion lost event order", input.Text())
	}
}

func TestSecondaryPointerCannotReleasePrimaryPress(t *testing.T) {
	t.Parallel()

	activations := 0

	button := ui.NewButton("Run", func() { activations++ })

	view := surface(t, ui.Column(button))

	positionX, positionY := point(button)

	view.Dispatch(ui.Event{Kind: ui.PointerDown, Pointer: 1, Device: "touch", X: positionX, Y: positionY})

	view.Dispatch(ui.Event{Kind: ui.PointerDown, Pointer: 2, Device: "touch", X: positionX, Y: positionY})

	view.Dispatch(ui.Event{Kind: ui.PointerUp, Pointer: 2, Device: "touch", X: positionX, Y: positionY})

	if activations != 0 || !view.CapturesPointer() {
		t.Fatal("secondary pointer released the primary press")
	}

	view.Dispatch(ui.Event{Kind: ui.PointerUp, Pointer: 1, Device: "touch", X: positionX, Y: positionY})

	if activations != 1 || view.CapturesPointer() {
		t.Fatal("primary release did not finish exactly once", activations)
	}
}

func TestHidingModalRestoresFocus(t *testing.T) {
	t.Parallel()

	opener := ui.NewButton("Open", nil)

	dialog := ui.Column(ui.NewButton("Close", nil))

	dialog.SetHidden(true)

	view := surface(t, ui.Column(opener, dialog))

	view.SetFocus(opener)

	if err := view.PushModal(dialog); err != nil {
		t.Fatal(err)
	}

	paint(t, view)

	dialog.SetHidden(true)

	paint(t, view)

	if view.CapturesKeyboard() || view.Focused() != opener {
		t.Fatal("hidden modal kept focus scope")
	}
}

func TestRemovingModalAncestorDuringDispatch(t *testing.T) {
	t.Parallel()

	opener := ui.NewButton("Open", nil)

	closeButton := ui.NewButton("Remove owner", nil)

	dialog := ui.Column(closeButton)

	owner := ui.Column(dialog)

	dialog.SetHidden(true)

	view := surface(t, ui.Column(opener, owner))

	view.SetFocus(opener)

	if err := view.PushModal(dialog); err != nil {
		t.Fatal(err)
	}

	closeButton.OnEvent(func(event *ui.RoutedEvent) {
		if event.Event.Kind == ui.PointerUp {
			owner.Remove()
		}
	})

	click(view, closeButton)

	paint(t, view)

	if view.CapturesKeyboard() || view.Focused() != opener || view.Lookup(dialog.ID()) != nil {
		t.Fatal("removing modal ancestor during dispatch retained detached scope")
	}
}

func TestInvalidThemeAndLabelPreservePreviousValues(t *testing.T) {
	t.Parallel()

	button := ui.NewButton("Original", nil)

	view := surface(t, ui.Column(button))

	original := view.Theme()

	invalid := original

	invalid.ControlHeight = 0

	if err := view.SetTheme(invalid); !errors.Is(err, ui.ErrBounds) {
		t.Fatal("invalid metrics admitted", err)
	}

	invalid = original

	invalid.Foreground[0] = float32(math.NaN())

	if err := view.SetTheme(invalid); !errors.Is(err, ui.ErrBounds) {
		t.Fatal("nonfinite color admitted", err)
	}

	if view.Theme() != original {
		t.Fatal("invalid theme partially published")
	}

	if err := button.SetLabel(strings.Repeat("x", ui.MaxTextBytes+1), ""); !errors.Is(err, ui.ErrBounds) {
		t.Fatal("oversized label admitted", err)
	}

	if err := button.SetLabel("Changed", string([]byte{0xff})); !errors.Is(err, ui.ErrBounds) {
		t.Fatal("invalid description admitted", err)
	}

	for _, semantic := range view.Semantics() {
		if semantic.ID == button.ID() && semantic.Name != "Original" {
			t.Fatal("invalid label partially published", semantic.Name)
		}
	}

	paint(t, view)
}

func TestInvalidInputPlaceholderRejectedAtAdmission(t *testing.T) {
	t.Parallel()

	font, err := defaultfont.Load()
	if err != nil {
		t.Fatal(err)
	}

	input := ui.NewInput("Name", "", strings.Repeat("x", ui.MaxTextBytes+1), nil)

	if _, err := ui.New(ui.Column(input), font); !errors.Is(err, ui.ErrBounds) {
		t.Fatal("oversized placeholder admitted", err)
	}

	invalid := ui.NewInput("Name", "", string([]byte{0xff}), nil)

	if _, err := ui.New(ui.Column(invalid), font); !errors.Is(err, ui.ErrBounds) {
		t.Fatal("invalid placeholder admitted", err)
	}
}

func TestReadonlyCancelsCompositionWithoutTrappingFocus(t *testing.T) {
	t.Parallel()

	input := ui.NewInput("Name", "original", "", nil)

	next := ui.NewButton("Next", nil)

	view := surface(t, ui.Column(input, next))

	view.SetFocus(input)

	view.Dispatch(ui.Event{Kind: ui.CompositionStart})

	view.Dispatch(ui.Event{Kind: ui.InsertText, Text: "provisional"})

	input.SetReadOnly(true)

	if input.Text() != "original" {
		t.Fatal("readonly retained provisional text", input.Text())
	}

	key(view, "Tab")

	if view.Focused() != next {
		t.Fatal("readonly composition trapped Tab")
	}

	input.SetReadOnly(false)

	view.SetFocus(input)

	if view.Editing().Composing {
		t.Fatal("composition survived readonly transition")
	}
}

func TestModalRestorationSkipsRemovedOpener(t *testing.T) {
	t.Parallel()

	opener := ui.NewButton("Open", nil)
	fallback := ui.NewButton("Available", nil)
	dialogInput := ui.NewInput("Note", "", "", nil)
	dialog := ui.Column(dialogInput)

	dialog.SetHidden(true)

	view := surface(t, ui.Column(opener, fallback, dialog))

	view.SetFocus(opener)

	if err := view.PushModal(dialog); err != nil {
		t.Fatal(err)
	}

	opener.Remove()
	view.PopModal()
	paint(t, view)

	if view.Focused() != fallback || view.CapturesKeyboard() {
		t.Fatal("modal retained hidden focus after opener removal")
	}
}
