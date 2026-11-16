package main

import (
	"strings"
	"testing"
	"time"

	"github.com/karty-game/karty-webgpu/pkg/debug"
	"github.com/karty-game/karty-webgpu/pkg/engine"
	"github.com/karty-game/karty-webgpu/pkg/ui"
	"github.com/karty-game/karty-webgpu/pkg/ui/app"
)

func navigationApp(t *testing.T) *engine.App {
	t.Helper()

	app := engine.NewApp(engine.AppConfig{})

	t.Cleanup(func() {
		if err := app.Close(); err != nil {
			t.Error(err)
		}
	})

	return app
}

func TestNavigationUsesRetainedControlsAndSharesDebugView(t *testing.T) {
	t.Parallel()
	app := navigationApp(t)
	selected := -1

	navigation, err := showNavigation(app, func(index int) { selected = index })
	if err != nil {
		t.Fatal(err)
	}
	defer navigation.remove()

	view := uiapp.View(app)

	panel, err := debug.Show(app)
	if err != nil || uiapp.View(app) != view {
		t.Fatal("debug replaced the navigation View", err)
	}
	defer panel.Close()

	panel.Hide()

	for _, size := range [][2]float32{{1280, 720}, {360, 640}} {
		if err := view.Resize(size[0], size[1]); err != nil {
			t.Fatal(err)
		}

		if _, err := view.Frame(time.Millisecond); err != nil {
			t.Fatal(err)
		}
	}

	details := navigationControl(t, view, "Views")
	if result := view.Dispatch(ui.Event{Kind: ui.Activate, Target: details.ID()}); result.Err != nil {
		t.Fatal(result.Err)
	}

	if _, err := view.Frame(time.Millisecond); err != nil {
		t.Fatal(err)
	}

	for index, name := range []string{"Courtyard · 1", "Gallery · 2", "Free flight · 3", "Ceiling · B"} {
		button := navigationControl(t, view, name)
		if result := view.Dispatch(ui.Event{Kind: ui.Activate, Target: button.ID()}); result.Err != nil || selected != index {
			t.Fatal("retained camera control failed", name, selected, result.Err)
		}
	}

	for _, semantic := range view.Semantics() {
		if semantic.Name == "Lighting" {
			t.Fatal("navigation retained a lighting selector")
		}
	}

	if err := navigation.status.SetText("RNM lighting · 1/1 chunks ready"); err != nil {
		t.Fatal(err)
	}

	if navigation.status.Text() != "RNM lighting · 1/1 chunks ready" {
		t.Fatal("status did not update through retained text")
	}

	navigation.remove()

	if uiapp.View(app) != view || len(view.Semantics()) == 0 {
		t.Fatal("removing navigation detached the shared debug View")
	}

	for _, semantic := range view.Semantics() {
		if semantic.Name == "Courtyard · 1" {
			t.Fatal("navigation callbacks remained attached after removal")
		}
	}
}

func navigationControl(t *testing.T, view *ui.View, name string) *ui.Node {
	t.Helper()

	for _, semantic := range view.Semantics() {
		if semantic.Name == name {
			return view.Lookup(semantic.ID)
		}
	}

	t.Fatalf("missing retained control %q", name)

	return nil
}

func TestRNMPresentationKeepsCameraAndClosesWithApp(t *testing.T) {
	t.Parallel()
	app := navigationApp(t)
	world := engine.NewWorld()
	camera := world.Add(engine.DefaultTransform())
	transform := world.Transform(camera)
	notifications := 0
	text := ""

	presentation, err := openBakedScene(app, world)
	if err != nil {
		t.Fatal(err)
	}

	defer func() {
		if err := presentation.baked.Close(); err != nil {
			t.Error(err)
		}
	}()

	opened := presentation.baked
	presentation.changed = func(value string) { notifications++; text = value }
	presentation.notify(opened.Status())

	if opened.Status().Closed || notifications != 1 || !strings.HasPrefix(text, "RNM") {
		t.Fatal("initial prepared profile status missing", notifications, text)
	}

	if err := app.Close(); err != nil {
		t.Fatal(err)
	}

	if !opened.Status().Closed || world.Transform(camera) != transform || world.Len() != 1 || notifications != 1 {
		t.Fatal("App cleanup lost presentation lifetime, camera/world ownership or progress-only status")
	}

	if err := presentation.update(time.Millisecond); err != nil || presentation.baked != opened || notifications != 1 {
		t.Fatal("closed presentation restarted the loader or emitted status", err)
	}
}
