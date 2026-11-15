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
	selected, rgb := -1, false

	navigation, err := showNavigation(app, func(index int) { selected = index }, func(value bool) { rgb = value })
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

	details := navigationControl(t, view, "Views and lighting")
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

	lighting := navigationControl(t, view, "Lighting")
	view.SetFocus(lighting)

	for _, key := range []string{"Enter", "End", "Enter"} {
		if result := view.Dispatch(ui.Event{Kind: ui.KeyDown, Key: key}); result.Err != nil {
			t.Fatal(result.Err)
		}
	}

	if !rgb || lighting.Selected() != "rgb" {
		t.Fatal("lighting selection did not reach the sample")
	}

	if err := navigation.status.SetText("RGB · 1/1 chunks"); err != nil {
		t.Fatal(err)
	}

	if navigation.status.Text() != "RGB · 1/1 chunks" {
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

func TestLightingSwitchClosesOldPresentationAndKeepsWorld(t *testing.T) {
	t.Parallel()
	app := navigationApp(t)
	world := engine.NewWorld()
	camera := world.Add(engine.DefaultTransform())
	transform := world.Transform(camera)
	notifications := 0
	text := ""

	presentation := &bakedPresentation{app: app, world: world, changed: func(value string) { notifications++; text = value }}
	if err := presentation.applyLighting(); err != nil {
		t.Fatal(err)
	}

	defer func() {
		if err := presentation.baked.Close(); err != nil {
			t.Error(err)
		}
	}()

	first := presentation.baked

	if notifications != 1 || !strings.HasPrefix(text, "RNM") {
		t.Fatal("initial prepared profile status missing", notifications, text)
	}

	presentation.rgb = true
	if err := presentation.applyLighting(); err != nil {
		t.Fatal(err)
	}

	second := presentation.baked
	if !first.Status().Closed || second.Status().Closed || presentation.world != world || presentation.app != app ||
		world.Transform(
			camera,
		) != transform || world.Len() != 1 || !presentation.activeRGB || notifications != 2 || !strings.HasPrefix(text, "RGB") {
		t.Fatal("lighting switch lost cleanup, camera/world ownership or progress-only status")
	}

	if err := presentation.applyLighting(); err != nil || presentation.baked != second || notifications != 2 {
		t.Fatal("unchanged lighting restarted the loader", err)
	}

	presentation.rgb = false
	if err := presentation.applyLighting(); err != nil || !second.Status().Closed || presentation.activeRGB || notifications != 3 ||
		!strings.HasPrefix(text, "RNM") {
		t.Fatal("switching back did not close RGB and select RNM", err)
	}
}
