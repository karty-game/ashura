package ui_test

import (
	"errors"
	"testing"

	"github.com/karty-game/ashura/pkg/ui"
	"github.com/karty-game/ashura/pkg/ui/fonts/defaultfont"
)

func TestTabsKeyboardRovingFocusAndRetainedPanels(t *testing.T) {
	t.Parallel()

	changes := []string{}
	input := ui.NewInput("Note", "retained", "", nil)
	tabs := ui.NewTabs("Sections", []ui.Tab{
		{Key: "first", Label: "First", Content: input},
		{Key: "disabled", Label: "Disabled", Content: ui.NewText("Unavailable"), Disabled: true},
		{Key: "third", Label: "Third", Content: ui.NewButton("Panel action", nil)},
	}, "first", func(change ui.Change) { changes = append(changes, change.Text) })
	before, after := ui.NewButton("Before", nil), ui.NewButton("After", nil)
	view := surface(t, ui.Column(before, tabs, after))
	buttons := tabs.Children()[0].Children()

	view.SetFocus(before)
	key(view, "Tab")

	if view.Focused() != buttons[0] {
		t.Fatal("Tab did not enter the selected tab")
	}

	key(view, "ArrowRight")
	paint(t, view)

	if tabs.ActiveTab() != "third" || view.Focused() != buttons[2] || len(changes) != 1 || changes[0] != "third" {
		t.Fatal("arrow navigation failed to skip disabled tab", tabs.ActiveTab(), changes)
	}

	key(view, "ArrowRight")
	paint(t, view)
	key(view, "Tab")

	if tabs.ActiveTab() != "first" || view.Focused() != input {
		t.Fatal("tab wrap or roving Tab exit failed")
	}

	view.Dispatch(ui.Event{Kind: ui.InsertText, Text: " value"})

	if err := tabs.SetActiveTab("third"); err != nil {
		t.Fatal(err)
	}

	paint(t, view)

	if view.Focused() != buttons[2] || len(changes) != 2 {
		t.Fatal("silent panel switch did not hand focus to its tab")
	}

	if err := tabs.SetActiveTab("first"); err != nil {
		t.Fatal(err)
	}

	paint(t, view)

	if input.Text() != "retained value" || len(changes) != 2 {
		t.Fatal("hidden panel state was lost or setter notified", input.Text(), changes)
	}
}

func TestTabsHomeEndAndSemanticRelationships(t *testing.T) {
	t.Parallel()

	tabs := ui.NewTabs("Sections", []ui.Tab{
		{Key: "first", Label: "First", Content: ui.NewText("Read this panel")},
		{Key: "last", Label: "Last", Content: ui.NewText("Read another panel")},
	}, "last", nil)
	view := surface(t, ui.Column(tabs, ui.NewButton("After", nil)))
	buttons := tabs.Children()[0].Children()
	view.SetFocus(buttons[1])
	key(view, "Home")
	paint(t, view)

	if tabs.ActiveTab() != "first" || view.Focused() != buttons[0] {
		t.Fatal("Home did not select the first tab")
	}

	key(view, "End")
	paint(t, view)
	key(view, "Tab")

	if tabs.ActiveTab() != "last" || view.Focused() != tabs.Children()[2] {
		t.Fatal("static panel could not receive keyboard focus")
	}

	roles := map[ui.ID]ui.Semantic{}
	for _, semantic := range view.Semantics() {
		roles[semantic.ID] = semantic
	}

	if roles[buttons[1].ID()].Role != "tab" || !roles[buttons[1].ID()].Selected || roles[tabs.Children()[0].ID()].Role != "tablist" {
		t.Fatal("tab semantic role/selection mismatch")
	}

	selected := roles[buttons[1].ID()]

	panel := roles[tabs.Children()[2].ID()]
	if selected.Controls != panel.ID || panel.LabelledBy != selected.ID || panel.Role != "tabpanel" {
		t.Fatal("tab/panel semantic relationships missing", selected, panel)
	}

	if err := tabs.SetActiveTab("missing"); !errors.Is(err, ui.ErrBounds) || tabs.ActiveTab() != "last" {
		t.Fatal("invalid tab changed selection", err)
	}
}

func TestTabsRejectInvalidConstructionWithoutAdoptingContent(t *testing.T) {
	t.Parallel()

	content := ui.NewText("Reusable after failed construction")
	invalid := ui.NewTabs("Sections", []ui.Tab{
		{Key: "same", Label: "First", Content: content},
		{Key: "same", Label: "Second", Content: ui.NewText("Other")},
	}, "same", nil)

	font, err := defaultfont.Load()
	if err != nil {
		t.Fatal(err)
	}

	if _, err := ui.New(invalid, font); !errors.Is(err, ui.ErrBounds) {
		t.Fatal("duplicate tab keys accepted", err)
	}

	root := ui.Column(content)
	if _, err := ui.New(root, font); err != nil {
		t.Fatal("failed construction adopted caller content", err)
	}
}

func TestPaginationBoundsAndSilentSetters(t *testing.T) {
	t.Parallel()

	changes := 0
	pagination := ui.NewPagination("Results", 1, 10, func(change ui.Change) {
		changes++

		if change.Value != float64(change.Node.Page()) {
			t.Error("change has wrong numeric page")
		}
	})
	view := surface(t, ui.Column(pagination))
	navigation := pagination.Children()[1].Children()
	click(view, navigation[2])
	paint(t, view)

	if pagination.Page() != 2 || changes != 1 {
		t.Fatal("Next did not advance once", pagination.Page(), changes)
	}

	if err := pagination.SetPage(9); err != nil {
		t.Fatal(err)
	}

	paint(t, view)
	click(view, navigation[3])
	paint(t, view)

	if pagination.Page() != 10 || changes != 2 || view.Focused() == nil {
		t.Fatal("Last or boundary focus handoff failed", pagination.Page(), changes)
	}

	currentPages := 0

	for _, semantic := range view.Semantics() {
		if semantic.Current == "page" {
			currentPages++

			if semantic.Name != "Page 10" {
				t.Fatal("wrong current page semantics", semantic)
			}
		}
	}

	if currentPages != 1 {
		t.Fatal("pagination did not expose one current page", currentPages)
	}

	click(view, navigation[2])

	if pagination.Page() != 10 || changes != 2 {
		t.Fatal("disabled Next activated")
	}

	if err := pagination.SetPage(11); !errors.Is(err, ui.ErrBounds) || pagination.Page() != 10 {
		t.Fatal("out-of-range page accepted", err)
	}

	if err := pagination.SetPageCount(3); err != nil {
		t.Fatal(err)
	}

	paint(t, view)

	if pagination.Page() != 3 || pagination.PageCount() != 3 || changes != 2 {
		t.Fatal("page count did not clamp silently")
	}
}

func TestPaginationHugeCountsUseBoundedNodes(t *testing.T) {
	t.Parallel()

	pagination := ui.NewPagination("Results", 500_000_000, 1_000_000_000, nil)
	view := surface(t, ui.Column(pagination))
	before := view.Stats().Nodes

	if before > 16 {
		t.Fatal("pagination allocated by page count", before)
	}

	for _, page := range []int{1, 999_999_999, 1_000_000_000, 5} {
		if err := pagination.SetPage(page); err != nil {
			t.Fatal(err)
		}

		paint(t, view)
	}

	if view.Stats().Nodes != before {
		t.Fatal("page updates replaced retained navigation controls")
	}
}

func TestTabsRepairDisabledHiddenAndRemovedSelection(t *testing.T) {
	t.Parallel()

	changes := 0
	tabs := ui.NewTabs("Sections", []ui.Tab{
		{Key: "one", Label: "One", Content: ui.NewText("First")},
		{Key: "two", Label: "Two", Content: ui.NewText("Second")},
	}, "one", func(ui.Change) { changes++ })
	view := surface(t, ui.Column(tabs, ui.NewButton("After", nil)))
	buttons := tabs.Children()[0].Children()
	view.SetFocus(buttons[0])
	buttons[0].SetDisabled(true)
	paint(t, view)

	if tabs.ActiveTab() != "two" || view.Focused() != buttons[1] {
		t.Fatal("disabled selected tab left a broken roving stop")
	}

	buttons[1].SetHidden(true)
	paint(t, view)

	if tabs.ActiveTab() != "" || view.Focused() != nil {
		t.Fatal("all unavailable tabs retained selection/focus")
	}

	buttons[0].SetDisabled(false)
	paint(t, view)
	key(view, "Tab")

	if tabs.ActiveTab() != "one" || view.Focused() != buttons[0] || changes != 0 {
		t.Fatal("re-enabling did not restore silent roving selection")
	}

	buttons[1].SetHidden(false)
	tabs.Children()[1].Remove()
	paint(t, view)

	if tabs.ActiveTab() != "two" || view.Focused() != buttons[1] {
		t.Fatal("removing active panel retained a stale selection")
	}
}

func TestTabChangeCallbackMayRemoveGroup(t *testing.T) {
	t.Parallel()

	var tabs *ui.Node

	tabs = ui.NewTabs("Sections", []ui.Tab{
		{Key: "one", Label: "One", Content: ui.NewText("First")},
		{Key: "two", Label: "Two", Content: ui.NewText("Second")},
	}, "one", func(ui.Change) { tabs.Remove() })
	view := surface(t, ui.Column(tabs, ui.NewButton("After", nil)))
	view.SetFocus(tabs.Children()[0].Children()[0])
	key(view, "ArrowRight")
	paint(t, view)

	if view.Lookup(tabs.ID()) != nil || view.Focused() != nil {
		t.Fatal("tab callback removal left stale state")
	}
}
