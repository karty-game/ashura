package engine

import (
	"testing"
	"time"

	"github.com/karty-game/ashura/internal/platform"
	"github.com/karty-game/ashura/internal/render"
	"github.com/karty-game/ashura/pkg/compute"
	"github.com/karty-game/ashura/pkg/shader"
	"github.com/karty-game/ashura/pkg/texture"
	"github.com/karty-game/ashura/pkg/ui"
	"github.com/karty-game/ashura/pkg/ui/fonts/defaultfont"
)

//nolint:paralleltest // AllocsPerRun must be isolated on Go 1.27.
func TestAppUpdateFrameDoesNotAllocate(t *testing.T) {
	app := NewApp(AppConfig{})
	defer func() { _ = app.Close() }()

	attachAllocationTestUI(t, app)

	image, err := app.NewTexture(texture.Data{
		Width: 1, Height: 1, ColorSpace: texture.SRGB, Levels: [][]byte{{255, 255, 255, 255}},
	})
	if err != nil {
		t.Fatal(err)
	}

	var parameters [16]byte

	uniform, err := shader.NewUniformBytes(app, parameters[:])
	if err != nil {
		t.Fatal(err)
	}

	kernel, err := compute.NewPipeline(app, compute.PipelineDesc{
		Program: shader.WGSL("wgsl/compute-fixture.wgsl", scheduleWGSL), EntryPoint: "main",
		Layout: shader.Layout{shader.UniformLayout(0, 16)},
	})
	if err != nil {
		t.Fatal(err)
	}

	group, err := kernel.BindGroup(shader.BindBuffer(0, uniform))
	if err != nil {
		t.Fatal(err)
	}

	world := NewWorld()
	camera := world.Add(DefaultTransform(), cameraFixture{})
	geometry := fixtureTriangle()
	source := new(int)

	var moving Entity
	for range 1000 {
		moving = world.Add(DefaultTransform(), instanceFixture{geometryFixture: geometry, source: source}, textureFixture{image: image})
	}

	var writeErr error

	app.OnUpdate(func(delta time.Duration) {
		world.Transform(camera).Position.X += float32(delta.Seconds())
		world.Transform(moving).Position.Y += float32(delta.Seconds())
		parameters[0]++
		writeErr = uniform.Write(0, parameters[:])
	})
	app.OnCompute(func(frame *Frame) error { return frame.Dispatch(kernel, group, 1, 1, 1) })

	input := platform.InputState{Width: 640, Height: 480, PointerLocked: true, MouseDX: .1}
	input.Keys[platform.KeyCodeW] = true

	var frame render.Frame
	for range 3 {
		frame, err = app.updateFrame(world, time.Second/60, input)
		if err != nil || writeErr != nil {
			t.Fatalf("warm update errors: %v, %v", err, writeErr)
		}
	}

	beforeProjection := frame.ViewProjection
	beforeModel := frame.Draws[0].Instances[999].Model

	allocations := testing.AllocsPerRun(100, func() {
		frame, err = app.updateFrame(world, time.Second/60, input)
	})
	if allocations != 0 || err != nil || writeErr != nil {
		t.Fatalf("App update allocations=%v, update error=%v, buffer error=%v", allocations, err, writeErr)
	}

	if frame.ViewProjection == beforeProjection || frame.Draws[0].Instances[999].Model == beforeModel {
		t.Fatal("update loop stopped camera/object movement")
	}

	if len(frame.Compute) != 1 || len(frame.Uploads) != 1 || frame.Uploads[0].Data[0] != parameters[0] {
		t.Fatal("compute scheduling or same-frame buffer writes stopped during allocation measurement")
	}

	if frame.Draws[0].Albedo != image || !app.Input().KeyDown(KeyW) {
		t.Fatal("texture ownership/normalized input path was not exercised")
	}

	if frame.UI == nil || len(frame.UI.Data) == 0 {
		t.Fatal("retained UI composition was not exercised")
	}
}

func attachAllocationTestUI(t *testing.T, app *App) {
	t.Helper()

	font, err := defaultfont.Load()
	if err != nil {
		t.Fatal(err)
	}

	view, err := ui.New(ui.Column(ui.NewText("Retained allocation fixture")), font)
	if err != nil {
		t.Fatal(err)
	}

	if err := app.SetUI(&testUIAttachment{View: view}); err != nil {
		t.Fatal(err)
	}
}
