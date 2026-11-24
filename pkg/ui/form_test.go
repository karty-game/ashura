package ui_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/karty-game/ashura/pkg/ui"
	"github.com/karty-game/ashura/pkg/ui/fonts/defaultfont"
)

func TestFieldAssociatesLabelHintAndError(t *testing.T) {
	t.Parallel()

	changes := 0

	control := ui.NewInput("Original", "Explorer", "", func(ui.Change) { changes++ })
	if err := control.SetLabel("Original", "Existing service description"); err != nil {
		t.Fatal(err)
	}

	field := ui.NewField("Account name", control)
	if err := field.SetHelpText("Use the name other players will see."); err != nil {
		t.Fatal(err)
	}

	view := surface(t, ui.NewForm(field))
	view.SetFocus(control)

	parts := field.Children()

	semantic := semanticByID(t, view, control.ID())
	if semantic.Name != "Account name" || semantic.LabelledBy != parts[0].ID() || semantic.DescribedBy != parts[2].ID() ||
		semantic.Invalid || semantic.Description != "Existing service description" {
		t.Fatal("field did not associate its label/hint", semantic)
	}

	if err := field.SetErrorText("Choose a longer account name."); err != nil {
		t.Fatal(err)
	}

	paint(t, view)

	semantic = semanticByID(t, view, control.ID())
	if !semantic.Invalid || semantic.DescribedBy != parts[3].ID() || view.Focused() != control || changes != 0 ||
		control.Text() != "Explorer" || parts[3].Bounds().Y <= parts[2].Bounds().Y {
		t.Fatal("error update changed editing state or missed reflow", semantic)
	}

	beforeRepeat := view.Stats()

	if err := field.SetErrorText("Choose a longer account name."); err != nil {
		t.Fatal(err)
	}

	paint(t, view)

	if view.Stats() != beforeRepeat {
		t.Fatal("unchanged validation state repainted the form")
	}

	if err := field.SetErrorText(""); err != nil {
		t.Fatal(err)
	}

	paint(t, view)

	semantic = semanticByID(t, view, control.ID())
	if semantic.Invalid || semantic.DescribedBy != parts[2].ID() || view.Focused() != control {
		t.Fatal("clearing error did not restore hint", semantic)
	}

	if err := field.SetHelpText(""); err != nil {
		t.Fatal(err)
	}

	paint(t, view)

	if semanticByID(t, view, control.ID()).DescribedBy != 0 {
		t.Fatal("clearing hint left a stale description relation")
	}
}

func TestFieldValidationPreservesControlOwnership(t *testing.T) {
	t.Parallel()

	font, err := defaultfont.Load()
	if err != nil {
		t.Fatal(err)
	}

	control := ui.NewInput("Original", "value", "", nil)

	invalid := ui.NewField(strings.Repeat("x", ui.MaxTextBytes+1), control)
	if _, err := ui.New(invalid, font); !errors.Is(err, ui.ErrBounds) {
		t.Fatal("oversized field label accepted", err)
	}

	field := ui.NewField("Valid", control)

	view := surface(t, ui.NewForm(field))
	if semanticByID(t, view, control.ID()).Name != "Valid" {
		t.Fatal("failed constructor adopted or modified the original control")
	}

	if err := field.SetHelpText("Useful hint"); err != nil {
		t.Fatal(err)
	}

	if err := field.SetHelpText(string([]byte{0xff})); !errors.Is(err, ui.ErrBounds) {
		t.Fatal("invalid UTF-8 hint accepted", err)
	}

	if err := field.SetErrorText(strings.Repeat("x", ui.MaxTextBytes+1)); !errors.Is(err, ui.ErrBounds) {
		t.Fatal("oversized error accepted", err)
	}

	paint(t, view)

	if semanticByID(t, view, control.ID()).Invalid || field.Children()[2].Text() != "Useful hint" {
		t.Fatal("invalid updates changed field state")
	}

	if err := ui.Column().SetHelpText("Hint"); !errors.Is(err, ui.ErrBounds) {
		t.Fatal("field setter accepted a plain panel", err)
	}

	control.Remove()

	if err := field.SetErrorText("Detached"); !errors.Is(err, ui.ErrBounds) {
		t.Fatal("field setter targeted an independently detached control", err)
	}
}

func TestWrappingFormUsesConstraintsInsteadOfCoordinates(t *testing.T) {
	t.Parallel()

	first := ui.NewField("First name", ui.NewInput("", "Ada", "", nil))
	second := ui.NewField("Last name", ui.NewInput("", "Lovelace", "", nil))

	view := surface(t, ui.NewForm(ui.Row(first, second).WithStyle(ui.Style{Wrap: true, Gap: 16})))
	if first.Bounds().Y != second.Bounds().Y || second.Bounds().X <= first.Bounds().X {
		t.Fatal("wide form did not share a row", first.Bounds(), second.Bounds())
	}

	if err := view.Resize(300, 600); err != nil {
		t.Fatal(err)
	}

	paint(t, view)

	if second.Bounds().Y <= first.Bounds().Y || first.Bounds().W > 300 || second.Bounds().W > 300 {
		t.Fatal("narrow form failed to wrap/shrink", first.Bounds(), second.Bounds())
	}

	if err := first.SetHelpText("This hint wraps naturally within the field rather than overlapping the next row."); err != nil {
		t.Fatal(err)
	}

	paint(t, view)

	if second.Bounds().Y < first.Bounds().Y+first.Bounds().H {
		t.Fatal("multiline hint overlapped another field", first.Bounds(), second.Bounds())
	}
}

func TestFieldPreservesBooleanInlineLabel(t *testing.T) {
	t.Parallel()

	checkbox := ui.NewCheckbox("I agree to the terms", false, nil)
	view := surface(t, ui.NewForm(ui.NewField("Consent", checkbox)))

	semantic := semanticByID(t, view, checkbox.ID())
	if semantic.Name != "I agree to the terms" || semantic.LabelledBy != 0 {
		t.Fatal("field caption overwrote the boolean control's inline label")
	}
}

func semanticByID(t *testing.T, view *ui.View, identity ui.ID) ui.Semantic {
	t.Helper()

	for _, semantic := range view.Semantics() {
		if semantic.ID == identity {
			return semantic
		}
	}

	t.Fatal("semantic not found", identity)

	return ui.Semantic{}
}
