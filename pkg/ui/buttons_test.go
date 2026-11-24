package ui_test

import (
	"testing"

	"github.com/karty-game/ashura/pkg/ui"
)

func TestButtonPriorityPreservesInteractionAndPaintOnly(t *testing.T) {
	t.Parallel()

	count := 0
	button := ui.NewButton("Save", func() { count++ })

	view := surface(t, ui.Column(button))
	if button.ButtonVariant() != ui.SecondaryButton {
		t.Fatal("default is not secondary")
	}

	before := view.Stats()

	if err := button.SetButtonVariant(ui.PrimaryButton); err != nil {
		t.Fatal(err)
	}

	paint(t, view)

	if view.Stats().Measures != before.Measures || view.Stats().Shapes != before.Shapes {
		t.Fatal("priority change affected layout")
	}

	click(view, button)
	key(view, "Enter")

	if count != 2 {
		t.Fatal("priority changed activation", count)
	}

	if err := button.SetButtonVariant(ui.ButtonVariant(255)); err == nil || button.ButtonVariant() != ui.PrimaryButton {
		t.Fatal("invalid variant mutated the button")
	}
}
