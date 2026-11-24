package debug_test

import (
	"errors"
	"testing"

	"github.com/karty-game/ashura/pkg/debug"
	"github.com/karty-game/ashura/pkg/engine"
	"github.com/karty-game/ashura/pkg/ui"
	"github.com/karty-game/ashura/pkg/ui/app"
	"github.com/karty-game/ashura/pkg/ui/fonts/defaultfont"
)

func TestPanelSharesApplicationUIAndRemovesOnlyItsOwnNodes(t *testing.T) {
	t.Parallel()

	app, view, action := applicationUI(t)
	before := view.Stats().Nodes
	root := view.Root()

	panel, err := debug.Show(app)
	if err != nil {
		t.Fatal(err)
	}

	if uiapp.View(app) != view || view.Root() != root || len(root.Children()) != 1 || !view.PreventsPointerLock() {
		t.Fatal("debug panel replaced the application UI or failed to request its cursor")
	}

	frameUI(t, view)
	activate(t, view, "Hide debug panel")

	if panel.Active() || view.PreventsPointerLock() {
		t.Fatal("Hide did not suspend collection and release its cursor request")
	}

	frameUI(t, view)
	activate(t, view, "Show debug panel")

	if !panel.Active() || !view.PreventsPointerLock() {
		t.Fatal("Debug button did not restore its retained overlay")
	}

	app.SetFrameObserver(nil)

	if panel.Active() || view.Stats().Nodes != before || view.PreventsPointerLock() || panel.Err() != nil {
		t.Fatal("observer replacement retained debug nodes, cursor state or an error")
	}

	panel.Show()

	result := view.Dispatch(ui.Event{Kind: ui.Activate, Target: action.ID()})
	if result.Err != nil || !result.Consumed || action.Text() != "Application action called" {
		t.Fatal("removing debug UI damaged the original application control", result)
	}
}

func TestPanelShortcutsWorkWithoutFocusAndAreRemovedOnClose(t *testing.T) {
	t.Parallel()

	app, view, _ := applicationUI(t)

	panel, err := debug.Show(app)
	if err != nil {
		t.Fatal(err)
	}

	frameUI(t, view)

	if view.Focused() != nil {
		t.Fatal("debug setup unexpectedly moved application focus")
	}

	for _, event := range []ui.Event{
		{Kind: ui.KeyDown, Key: "F3"},
		{Kind: ui.KeyDown, Key: "F3", Repeat: true},
	} {
		result := view.Dispatch(event)
		if result.Err != nil || !result.Consumed || panel.Active() {
			t.Fatal("F3 did not hide once and suppress repeats", result)
		}
	}

	view.Dispatch(ui.Event{Kind: ui.KeyDown, Key: "F3"})

	if !panel.Active() {
		t.Fatal("F3 did not reopen the panel without focus")
	}

	result := view.Dispatch(ui.Event{Kind: ui.KeyDown, Key: "Escape"})
	if result.Err != nil || !result.Consumed || panel.Active() {
		t.Fatal("Escape did not hide the visible panel", result)
	}

	panel.Close()

	result = view.Dispatch(ui.Event{Kind: ui.KeyDown, Key: "F3"})
	if result.Err != nil || result.Consumed || panel.Active() {
		t.Fatal("closed panel retained its global shortcut", result)
	}
}

func TestPanelRejectsClosedApplicationAndPreservesFullApplicationUI(t *testing.T) {
	t.Parallel()

	app, view, _ := applicationUI(t)

	children := make([]*ui.Node, ui.MaxNodes-view.Stats().Nodes)
	for index := range children {
		children[index] = ui.NewText("Application content")
	}

	if err := view.Root().Add(children...); err != nil {
		t.Fatal(err)
	}

	before := view.Stats().Nodes
	if panel, err := debug.Show(app); panel != nil || !errors.Is(err, ui.ErrBounds) || view.Stats().Nodes != before {
		t.Fatalf("failed debug admission changed a full application surface: panel=%v, err=%v", panel, err)
	}

	if err := app.Close(); err != nil {
		t.Fatal(err)
	}

	if panel, err := debug.Show(app); panel != nil || !errors.Is(err, engine.ErrClosed) {
		t.Fatalf("closed app admitted a debug panel: panel=%v, err=%v", panel, err)
	}
}

func TestPanelEscapeDefersToApplicationKeyboardScope(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		open func(*testing.T, *ui.View)
	}{
		{"modal", func(t *testing.T, view *ui.View) {
			t.Helper()

			modal := ui.Column(ui.NewButton("Application dialog action", nil))
			modal.SetHidden(true)

			if err := view.Root().Add(modal); err != nil {
				t.Fatal(err)
			}

			if err := view.PushModal(modal); err != nil {
				t.Fatal(err)
			}
		}},
		{"popup", func(t *testing.T, view *ui.View) {
			t.Helper()

			selection := ui.NewSelect("Application preset", []ui.Option{{Key: "one", Label: "One"}}, "one", nil)
			if err := view.Root().Add(selection); err != nil {
				t.Fatal(err)
			}

			frameUI(t, view)

			result := view.Dispatch(ui.Event{Kind: ui.Activate, Target: selection.ID()})
			if result.Err != nil || !result.Consumed {
				t.Fatal("application popup did not open", result)
			}
		}},
	}

	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			app, view, _ := applicationUI(t)

			panel, err := debug.Show(app)
			if err != nil {
				t.Fatal(err)
			}

			frameUI(t, view)
			test.open(t, view)

			if !view.CapturesKeyboard() {
				t.Fatal("application keyboard scope is not active")
			}

			result := view.Dispatch(ui.Event{Kind: ui.KeyDown, Key: "Escape"})
			if result.Err != nil || !result.Consumed || view.CapturesKeyboard() || !panel.Active() {
				t.Fatal("first Escape must close the application scope and retain debug", result)
			}

			result = view.Dispatch(ui.Event{Kind: ui.KeyDown, Key: "Escape"})
			if result.Err != nil || !result.Consumed || panel.Active() {
				t.Fatal("next Escape must hide debug after the application scope ends", result)
			}
		})
	}
}

func applicationUI(t *testing.T) (*engine.App, *ui.View, *ui.Node) {
	t.Helper()

	font, err := defaultfont.Load()
	if err != nil {
		t.Fatal(err)
	}

	var action *ui.Node

	action = ui.NewButton("Application action", func() {
		if err := action.SetText("Application action called"); err != nil {
			t.Error(err)
		}
	})

	view, err := ui.New(ui.Column(action), font)
	if err != nil {
		t.Fatal(err)
	}

	app := engine.NewApp(engine.AppConfig{})

	t.Cleanup(func() {
		if err := app.Close(); err != nil {
			t.Error(err)
		}
	})

	if err := uiapp.Attach(app, view); err != nil {
		t.Fatal(err)
	}

	return app, view, action
}

func frameUI(t *testing.T, view *ui.View) {
	t.Helper()

	if err := view.Resize(800, 600); err != nil {
		t.Fatal(err)
	}

	if _, err := view.Frame(0); err != nil {
		t.Fatal(err)
	}
}

func activate(t *testing.T, view *ui.View, name string) {
	t.Helper()

	for _, node := range view.Semantics() {
		if node.Name == name && node.Role == "button" {
			result := view.Dispatch(ui.Event{Kind: ui.Activate, Target: node.ID})
			if result.Err != nil || !result.Consumed {
				t.Fatal("debug button could not be activated", name, result)
			}

			return
		}
	}

	t.Fatal("debug button was not present in retained UI semantics", name)
}
