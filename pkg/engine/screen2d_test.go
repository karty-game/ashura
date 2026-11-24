package engine

import (
	"encoding/binary"
	"math"
	"testing"
	"time"

	"github.com/karty-game/ashura/internal/platform"
	"github.com/karty-game/ashura/internal/spritepaint"
	"github.com/karty-game/ashura/pkg/draw2d"
	"github.com/karty-game/ashura/pkg/ui"
	"github.com/karty-game/ashura/pkg/ui/fonts/defaultfont"
)

func TestScreen2DAppLifetimeAndCameraFreeFrame(t *testing.T) {
	t.Parallel()

	app := NewApp(AppConfig{Width: 640, Height: 360})
	defer func() { _ = app.Close() }()

	world := NewWorld()
	frame, err := app.updateFrame(world, time.Second/60, platform.InputState{})

	if err != nil || frame.Screen2D != nil || app.screen2D != nil {
		t.Fatal("unused sprite layer was created", err)
	}

	screen := app.Screen2D()
	if screen != app.Screen2D() {
		t.Fatal("App returned a different sprite layer")
	}

	if width, height := screen.Size(); width != 640 || height != 360 {
		t.Fatal("initial layer did not use the configured drawable size", width, height)
	}

	sprite := screen.NewSprite(screenImage(t)).SetPosition(10, 10)
	frame, err = app.updateFrame(world, time.Second/60, platform.InputState{Width: 1280, Height: 720})

	if err != nil || frame.Screen2D == nil || len(frame.Screen2D.Batches) != 1 || len(frame.Draws) != 0 {
		t.Fatal("2D-only frame requires world content", err)
	}

	if frame.Screen2D.Width != 1280 || frame.Screen2D.Height != 720 || len(frame.Screen2D.Data) != spritepaint.Stride {
		t.Fatal("sprite layer uses a logical/world resolution", frame.Screen2D)
	}

	values := screenRecord(frame.Screen2D.Data)
	if values[4] != 10 || values[5] != 10 || values[6] != 32 || values[7] != 32 {
		t.Fatal("output resize moved or resized the sprite", values)
	}

	if err := app.Close(); err != nil {
		t.Fatal(err)
	}

	sprite.Destroy()
	sprite.Destroy()
	screenPanic(t, func() { sprite.SetPosition(0, 0) })
	screenPanic(t, func() { app.Screen2D() })

	var absent *App

	screenPanic(t, func() { absent.Screen2D() })
}

func TestScreen2DResizeIsVisibleDuringUpdate(t *testing.T) {
	t.Parallel()

	app := NewApp(AppConfig{})
	defer func() { _ = app.Close() }()

	screen := app.Screen2D()
	sprite := screen.NewSprite(screenImage(t))

	app.OnUpdate(func(time.Duration) {
		if width, height := screen.Size(); width != 1200 || height != 900 {
			t.Fatal("update observed the previous viewport", width, height)
		}

		x, y := screen.FromLogical(10, 20, 600, 450)
		sprite.SetPosition(x, y)
	})

	frame, err := app.updateFrame(NewWorld(), time.Second/60, platform.InputState{Width: 1200, Height: 900})
	if err != nil {
		t.Fatal(err)
	}

	values := screenRecord(frame.Screen2D.Data)
	if values[4] != 20 || values[5] != 40 {
		t.Fatal("logical pointer conversion differs from output pixels", values)
	}
}

func TestScreen2DAndUIKeepIndependentRevisions(t *testing.T) {
	t.Parallel()

	app := NewApp(AppConfig{})
	defer func() { _ = app.Close() }()

	face, err := defaultfont.Load()
	if err != nil {
		t.Fatal(err)
	}

	node := ui.NewPanel().WithStyle(ui.Style{Width: 50, Height: 50, Fill: ui.Hex(0xff0000)})

	view, err := ui.New(node, face)
	if err != nil {
		t.Fatal(err)
	}

	if err := app.SetUI(&testUIAttachment{View: view}); err != nil {
		t.Fatal(err)
	}

	sprite := app.Screen2D().NewSprite(screenImage(t))
	world := NewWorld()
	input := platform.InputState{Width: 640, Height: 480, LogicalWidth: 320, LogicalHeight: 240}

	frame, err := app.updateFrame(world, time.Second/60, input)
	if err != nil {
		t.Fatal(err)
	}

	uiRevision, spriteRevision := frame.UI.Revision, frame.Screen2D.Revision

	sprite.SetPosition(25, 25)

	frame, err = app.updateFrame(world, time.Second/60, input)
	if err != nil || frame.UI.Revision != uiRevision || frame.Screen2D.Revision == spriteRevision {
		t.Fatal("moving a sprite invalidated the UI", err)
	}

	spriteRevision = frame.Screen2D.Revision

	if err := node.SetStyle(ui.Style{Width: 50, Height: 50, Fill: ui.Hex(0x00ff00)}); err != nil {
		t.Fatal(err)
	}

	frame, err = app.updateFrame(world, time.Second/60, input)
	if err != nil || frame.UI.Revision == uiRevision || frame.Screen2D.Revision != spriteRevision {
		t.Fatal("painting UI invalidated sprite records", err)
	}
}

func screenImage(t *testing.T) *draw2d.Image {
	t.Helper()

	image, err := draw2d.NewImage(32, 32, make([]byte, 32*32*4))
	if err != nil {
		t.Fatal(err)
	}

	return image
}

func screenRecord(data []byte) [20]float32 {
	var values [20]float32
	for index := range values {
		values[index] = math.Float32frombits(binary.LittleEndian.Uint32(data[index*4:]))
	}

	return values
}

func screenPanic(t *testing.T, action func()) {
	t.Helper()

	defer func() {
		if recover() == nil {
			t.Error("expected a programming-error panic")
		}
	}()

	action()
}
