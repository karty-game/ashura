package ui_test

import (
	"testing"

	"github.com/karty-game/karty-webgpu/pkg/ui"
)

func TestEditingCompositionGraphemesAndHistory(t *testing.T) {
	t.Parallel()

	changes := 0
	node := ui.NewInput("Name", "", "Type", func(ui.Change) { changes++ })
	view := surface(t, ui.Column(node))
	view.SetFocus(node)
	view.Dispatch(ui.Event{Kind: ui.InsertText, Text: "e\u0301👩‍👩‍👧‍👦"})
	paint(t, view)
	key(view, "Backspace")

	if node.Text() != "e\u0301" {
		t.Fatalf("split emoji cluster: %q", node.Text())
	}

	key(view, "Backspace")

	if node.Text() != "" {
		t.Fatalf("split combining cluster: %q", node.Text())
	}

	view.Dispatch(ui.Event{Kind: ui.KeyDown, Key: "z", Modifiers: ui.Modifiers{Control: true}})

	if node.Text() != "e\u0301" {
		t.Fatal("undo")
	}

	view.Dispatch(ui.Event{Kind: ui.CompositionStart})
	view.Dispatch(ui.Event{Kind: ui.InsertText, Text: "\u65e5\u672c"})
	key(view, "Enter")

	if !view.Editing().Composing {
		t.Fatal("Enter committed/canceled composition")
	}

	view.Dispatch(ui.Event{Kind: ui.CompositionCancel})

	if node.Text() != "e\u0301" {
		t.Fatal("composition cancellation failed", node.Text())
	}

	view.Dispatch(ui.Event{Kind: ui.CompositionStart})
	view.Dispatch(ui.Event{Kind: ui.InsertText, Text: "\u8a9e"})
	view.Dispatch(ui.Event{Kind: ui.CompositionEnd})

	if node.Text() != "e\u0301\u8a9e" || view.Editing().Composing {
		t.Fatal("composition commit failed", node.Text())
	}

	if changes == 0 {
		t.Fatal("no editing notifications")
	}
}
func TestEditingSessionAndReadonly(t *testing.T) {
	t.Parallel()

	first := ui.NewInput("A", "first", "", nil)
	second := ui.NewInput("B", "second", "", nil)
	view := surface(t, ui.Column(first, second))
	view.SetFocus(first)
	state := view.Editing()
	view.SetFocus(second)
	view.Dispatch(ui.Event{Kind: ui.InsertText, Text: "late", Session: state.Session, Revision: state.Revision})

	if second.Text() != "second" {
		t.Fatal("late clipboard changed another session")
	}

	second.SetReadOnly(true)
	view.Dispatch(ui.Event{Kind: ui.InsertText, Text: "bad"})
	key(view, "Backspace")

	if second.Text() != "second" {
		t.Fatal("readonly changed")
	}

	second.SetReadOnly(false)
	view.Dispatch(ui.Event{Kind: ui.ReplaceText, Start: 1, End: 100, Text: "bad"})

	if second.Text() != "second" {
		t.Fatal("invalid range admitted")
	}
}
func TestSemanticRolesAndDefaults(t *testing.T) {
	t.Parallel()

	checkbox := ui.NewCheckbox("Hints", true, nil)
	switchControl := ui.NewSwitch("Sound", false, nil)
	view := surface(t, ui.Column(checkbox, switchControl))

	roles := map[ui.ID]ui.Semantic{}
	for _, state := range view.Semantics() {
		roles[state.ID] = state
	}

	if roles[checkbox.ID()].Role != "checkbox" || roles[switchControl.ID()].Role != "switch" || roles[checkbox.ID()].Name != "Hints" ||
		!roles[checkbox.ID()].Checked {
		t.Fatal("semantic roles/states were inferred from visuals")
	}
}
