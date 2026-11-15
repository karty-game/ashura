package main

import (
	"math"
	"testing"
	"time"

	"github.com/karty-game/karty-webgpu/pkg/engine"
	"github.com/karty-game/karty-webgpu/pkg/ui"
	"github.com/karty-game/karty-webgpu/samples/sprites2d/generated/assets"
)

func expeditionFixture(t *testing.T) *expedition {
	t.Helper()

	app := engine.NewApp(engine.AppConfig{})

	t.Cleanup(func() {
		if err := app.Close(); err != nil {
			t.Error(err)
		}
	})

	images, err := assets.Load()
	if err != nil {
		t.Fatal(err)
	}

	study := newExpedition(app, images)
	if err := study.createInterface(); err != nil {
		t.Fatal(err)
	}

	study.screen.Resize(1280, 800)

	if err := study.update(0); err != nil {
		t.Fatal(err)
	}

	study.screen.Frame(1280, 800)

	return study
}

func TestPausedExpeditionRetainsSpriteRecordsAndResumesFrames(t *testing.T) {
	t.Parallel()

	study := expeditionFixture(t)
	study.setPaused(true)

	if err := study.update(0); err != nil {
		t.Fatal(err)
	}

	study.screen.Frame(1280, 800)
	before := study.screen.Stats()

	image := study.jelly.Image()
	for range 80 {
		if err := study.update(time.Second / 60); err != nil {
			t.Fatal(err)
		}

		study.screen.Frame(1280, 800)
	}

	after := study.screen.Stats()
	if study.elapsed != 0 || study.jelly.Image() != image || before.PackedRecords != after.PackedRecords ||
		before.BatchBuilds != after.BatchBuilds {
		t.Fatal("paused scene changed its animation or repacked sprite storage", before, after)
	}

	study.setPaused(false)

	if err := study.update(600 * time.Millisecond); err != nil {
		t.Fatal(err)
	}

	if study.jelly.Image() == image {
		t.Fatal("resuming did not advance the prepared animation frame")
	}

	scaleX, _ := study.fish[0].sprite.Scale()
	if scaleX >= 0 {
		t.Fatal("image-frame change lost mirrored fish placement")
	}

	if stats := study.screen.Stats(); stats.Sprites != before.Sprites || stats.Pages != before.Pages {
		t.Fatal("animation created sprites or reloaded pages")
	}
}

func TestScanAndDepthRemainUsableWhilePausedAndAfterResize(t *testing.T) {
	t.Parallel()

	study := expeditionFixture(t)
	study.setPaused(true)
	study.flat = true
	study.applyLayers()

	if study.nearReef.Visible() || study.particles[0].sprite.Visible() {
		t.Fatal("depth control left its layers visible")
	}

	study.scanned = true
	study.screen.Resize(2560, 1600)

	if err := study.update(0); err != nil {
		t.Fatal(err)
	}

	for _, ring := range study.rings {
		x, y, width, height, set := ring.Clip()
		if !set || x < 0 || y < 0 || width <= 0 || height <= 0 || x+width > 2560 || y+height > 1600 {
			t.Fatal("scan clipping did not track the resized framebuffer")
		}
	}

	study.scanned = false
	study.layoutScan()

	for _, ring := range study.rings {
		if _, _, _, _, set := ring.Clip(); set {
			t.Fatal("disabling scan retained a clip")
		}
	}

	study.flat = false
	study.applyLayers()
	study.setPalette(2)

	red, green, blue := study.jelly.Tint()
	if !study.nearReef.Visible() || red <= green || green <= blue {
		t.Fatal("paused controls did not apply")
	}
}

func TestTouchSonarConvertsLogicalCoordinatesAndRespectsUI(t *testing.T) {
	t.Parallel()
	study := expeditionFixture(t)
	study.screen.Resize(2560, 1600)

	if err := study.view.Resize(1280, 800); err != nil {
		t.Fatal(err)
	}

	if _, err := study.view.Frame(0); err != nil {
		t.Fatal(err)
	}

	study.view.SetFocus(study.motion)

	if result := study.view.Dispatch(ui.Event{Kind: ui.PointerDown, Device: "touch", Pointer: 1, X: 896, Y: 320}); result.Err != nil {
		t.Fatal(result.Err)
	}

	if study.sonarX != .7 || study.sonarY != .4 {
		t.Fatal("touch sonar did not convert a DPR2 pointer", study.sonarX, study.sonarY)
	}

	if study.view.Focused() != nil {
		t.Fatal("returning to the water kept a control's keyboard focus")
	}

	button := study.motion.Bounds()
	study.pointSonar(&ui.RoutedEvent{Event: ui.Event{Kind: ui.PointerDown, Device: "touch", X: button.X + 4, Y: button.Y + 4}})

	if study.sonarX != .7 || study.sonarY != .4 {
		t.Fatal("a control press moved sonar behind the UI")
	}
}

func TestDenseSceneSubmitsRealSpritesWithoutFragmentingAtlasBatches(t *testing.T) {
	t.Parallel()
	study := expeditionFixture(t)

	stats := study.screen.Stats()
	if stats.Sprites < 1000 || stats.Sprites > 2000 {
		t.Fatal("showcase density outside requested range", stats)
	}

	frame := study.screen.Frame(1280, 800)

	submitted := 0
	for _, batch := range frame.Batches {
		submitted += batch.Count
	}

	if submitted < 1000 {
		t.Fatal("sprite count was padded with hidden objects", submitted)
	}

	if len(frame.Batches) > 40 {
		t.Fatal("habitat fragmented contiguous atlas batches", len(frame.Batches))
	}

	pebbleX, pebbleY := study.seabed[0].sprite.Position()
	study.flat = true
	study.applyLayers()
	bubbleX, bubbleY := study.bubbles[0].sprite.Position()

	rotation := study.kelp[0].sprite.Rotation()

	rootX, rootY := study.kelp[0].sprite.Position()
	if err := study.update(time.Second); err != nil {
		t.Fatal(err)
	}

	afterX, afterY := study.bubbles[0].sprite.Position()
	if bubbleX != afterX || bubbleY != afterY || rotation != study.kelp[0].sprite.Rotation() {
		t.Fatal("hidden habitat continued animation")
	}

	afterX, afterY = study.seabed[0].sprite.Position()
	if pebbleX != afterX || pebbleY != afterY {
		t.Fatal("static seabed moved during animation")
	}

	study.flat = false
	study.applyLayers()

	study.dirty = true
	if err := study.update(time.Second); err != nil {
		t.Fatal(err)
	}

	rotationDelta := math.Abs(float64(study.kelp[0].sprite.Rotation() - rotation))

	afterRootX, afterRootY := study.kelp[0].sprite.Position()
	if rotationDelta == 0 || rotationDelta > .05 || afterRootX != rootX || afterRootY != rootY {
		t.Fatal("kelp must sway slowly around stationary roots", rotationDelta)
	}

	if after := study.screen.Stats(); after.Sprites != stats.Sprites || after.Pages != stats.Pages {
		t.Fatal("density change grew object or image pools")
	}
}
