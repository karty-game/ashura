package engine

import (
	"context"
	"errors"
	"image"
	"runtime"
	"time"

	"github.com/karty-game/karty-webgpu/internal/bufferdata"
	"github.com/karty-game/karty-webgpu/internal/diagnostics"
	"github.com/karty-game/karty-webgpu/internal/platform"
	"github.com/karty-game/karty-webgpu/internal/render"
	"github.com/karty-game/karty-webgpu/internal/texturedata"
	"github.com/karty-game/karty-webgpu/internal/uihost"
	"github.com/karty-game/karty-webgpu/pkg/compute"
	"github.com/karty-game/karty-webgpu/pkg/shader"
	"github.com/karty-game/karty-webgpu/pkg/texture"
)

var errAppClosed = ErrClosed

var errAppRunning = errors.New("engine: app is already running")

var errAppNotRunning = errors.New("engine: app is not running")

var errInvalidViewport = errors.New("engine: viewport dimensions must be positive")

var errNilContext = errors.New("engine: run context is nil")

const defaultRoughness float32 = 0.5

const defaultMSAASamples uint32 = 4

// AppConfig describes the window or browser canvas used by an App.
type AppConfig struct {
	// MSAASamples is 4 by default; 1 disables MSAA. Other values use the default.
	MSAASamples uint32
	Title       string
	Width       int
	Height      int
	CanvasID    string
}

// App owns the platform lifecycle and the shared update/render loop.
// App methods belong to the loop goroutine. Use RunContext cancellation to stop
// from another goroutine; Close may be called inside an OnUpdate callback.
type App struct {
	screen2D         screenLayer
	ui               uihost.Attachment
	features         []Feature
	featureMask      platform.FeatureMask
	uiTime           time.Duration
	metrics          diagnostics.Monitor
	buffers          map[*shader.Buffer]struct{}
	computePipelines map[*compute.Pipeline]struct{}
	computeGroups    map[*compute.BindGroup]struct{}
	dirtyBuffers     []*bufferdata.Buffer
	bufferUploads    []bufferdata.Upload
	computeUpdates   callbackList[func(*Frame) error]
	computeFrame     Frame
	computeActive    bool

	scene             *World
	textures          map[*texture.Texture]struct{}
	config            AppConfig
	updates           callbackList[func(time.Duration) error]
	closeCallbacks    map[uint64]func()
	nextCloseCallback uint64
	driver            platform.Driver
	input             platform.InputState
	closed            bool
}

// NewApp creates an application. The platform window or canvas is opened by Run.
func NewApp(config AppConfig) *App {
	if config.MSAASamples != 1 && config.MSAASamples != defaultMSAASamples {
		config.MSAASamples = defaultMSAASamples
	}

	if config.Width <= 0 {
		config.Width = 800
	}

	if config.Height <= 0 {
		config.Height = 600
	}

	return &App{config: config}
}

// Input returns the normalized input snapshot for the current frame.
func (app *App) Input() Input { return Input{state: app.input} }

// Input provides read-only access to normalized frame input.
type Input struct{ state platform.InputState }

// KeyDown reports whether the platform-neutral key is held.
func (input Input) KeyDown(key Key) bool { return input.state.Keys[uint8(key)] }

// MousePosition returns the pointer position in logical pixels.
func (input Input) MousePosition() Vector2 {
	return Vector2{X: input.state.MouseX, Y: input.state.MouseY}
}

// MouseDelta returns pointer movement since the prior frame.
func (input Input) MouseDelta() Vector2 {
	return Vector2{X: input.state.MouseDX, Y: input.state.MouseDY}
}

// Run opens the platform backend and runs updates and rendering until the
// native window is closed or the browser app is closed.
func (app *App) Run(world *World) error {
	return app.RunContext(context.Background(), world)
}

// RunContext runs the application until ctx is canceled or the app is closed.
// Browser builds schedule frames with requestAnimationFrame.
func (app *App) RunContext(ctx context.Context, world *World) error {
	if app == nil || app.closed {
		return errAppClosed
	}

	if app.driver != nil {
		return errAppRunning
	}

	if ctx == nil {
		return errNilContext
	}

	runtime.LockOSThread()

	defer runtime.UnlockOSThread()

	if err := ctx.Err(); err != nil {
		return err
	}

	if world == nil {
		world = NewWorld()
	}

	shadowResolution, err := pointShadowResolution(world)
	if err != nil {
		return err
	}

	if err := app.validateFeatureSetup(shadowResolution); err != nil {
		return err
	}

	driver, err := newDriver(platform.Config{
		UI:                    app.ui,
		Features:              app.features,
		PointShadowResolution: shadowResolution,
		Title:                 app.config.Title,
		SampleCount:           app.config.MSAASamples,
		Metrics:               &app.metrics,
		Width:                 app.config.Width,
		Height:                app.config.Height,
		CanvasID:              app.config.CanvasID,
	})
	if err != nil {
		return err
	}

	app.scene = world

	app.driver = driver
	defer func() {
		_ = app.Close()
	}()

	resources := app.computeResources()
	if len(resources.Buffers) > 0 || len(resources.Pipelines) > 0 || len(resources.BindGroups) > 0 {
		if err := driver.PrepareCompute(resources); err != nil {
			return err
		}
	}

	err = driver.Run(ctx, func(dt time.Duration, input platform.InputState) (render.Frame, error) {
		return app.updateFrame(world, dt, input)
	})
	if app.closed && errors.Is(err, errAppClosed) {
		return nil
	}

	return err
}

// Capture reserves the explicit screenshot API. Both current platform backends
// return an unsupported-capture error; frame readback is not implemented.
// The App must still be running and open.
func (app *App) Capture(ctx context.Context) (*image.RGBA, error) {
	if app == nil || app.closed {
		return nil, errAppClosed
	}

	if app.driver == nil {
		return nil, errAppNotRunning
	}

	return app.driver.Capture(ctx)
}

// Close releases platform resources. It is safe to call more than once.
func (app *App) Close() error {
	if app == nil || app.closed {
		return nil
	}

	app.closed = true
	app.updates.clear()
	app.runCloseCallbacks()

	if app.screen2D != nil {
		app.screen2D.Close()
	}

	app.closeFrameObserver()
	app.closeCompute()

	for image := range app.textures {
		texturedata.Release(image)
	}

	app.textures = nil

	app.scene = nil
	if app.driver == nil {
		return nil
	}

	err := app.driver.Close()
	app.driver = nil

	return err
}
