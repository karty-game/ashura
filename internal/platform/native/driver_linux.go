//go:build linux

// Package native provides the Linux window and WebGPU driver.
package native

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/Zyko0/go-sdl3/bin/binsdl"
	"github.com/Zyko0/go-sdl3/sdl"
	"github.com/gogpu/gputypes"
	"github.com/karty-game/karty-webgpu/internal/diagnostics"
	"github.com/karty-game/karty-webgpu/internal/impostordata"
	"github.com/karty-game/karty-webgpu/internal/platform"
	"github.com/karty-game/karty-webgpu/internal/render"
	"github.com/karty-game/karty-webgpu/internal/texturedata"
	wgpu "github.com/karty-game/karty-webgpu/internal/webgpu"
)

var (
	errQueueUnavailable   = errors.New("WebGPU device has no default queue")
	errCaptureUnsupported = errors.New("native screenshot capture is not implemented")
	errNoSurfaceFormats   = errors.New("WebGPU surface has no supported formats")
	errNoNativeWindow     = errors.New("SDL window uses neither X11 nor Wayland")
	errDriverClosed       = errors.New("native driver is closed")
	errEmptySurfaceFrame  = errors.New("WebGPU returned an empty surface texture")
)

const (
	float32Size          = 4
	allSampleMask uint32 = 0xFFFFFFFF
)

type sdlLibrary interface {
	Unload()
}

// Driver owns the SDL window and WebGPU presentation resources.
type Driver struct {
	uiBridge       nativeUIBridge
	spriteBridge   spriteBackend
	shadowBridge   shadowBackend
	computeBridge  computeBackendInterface
	pointShadow    *gpuPointShadow
	passColors     [1]wgpu.RenderPassColorAttachment
	passDepth      wgpu.RenderPassDepthStencilAttachment
	passDescriptor wgpu.RenderPassDescriptor
	queue          *wgpu.Queue
	metrics        *diagnostics.Monitor
	textureBytes   uint64
	compression    nativeCompression

	sampleCount          uint32
	colorTexture         *wgpu.Texture
	colorView            *wgpu.TextureView
	textures             map[*texturedata.Texture]gpuTexture
	geometry             render.GeometryCache[gpuGeometry]
	instances            map[uint32]*gpuInstances
	materials            map[uint32]*gpuMaterial
	impostorViews        map[*impostordata.Document]*gpuImpostor
	impostorSampler      *wgpu.Sampler
	white, flat, whiteAO *texturedata.Texture
	sampler              *wgpu.Sampler
	lightmapSampler      *wgpu.Sampler
	navigation           nativeInput

	sdlLibrary             sdlLibrary
	window                 *sdl.Window
	instance               *wgpu.Instance
	adapter                *wgpu.Adapter
	device                 *wgpu.Device
	surface                *wgpu.Surface
	depthTexture           *wgpu.Texture
	depthView              *wgpu.TextureView
	depthWidth             uint32
	depthHeight            uint32
	standardPipeline       *wgpu.RenderPipeline
	standardCulledPipeline *wgpu.RenderPipeline
	sourceCulledPipeline   *wgpu.RenderPipeline
	sourcePipeline         *wgpu.RenderPipeline
	impostorPipeline       *wgpu.RenderPipeline
	format                 gputypes.TextureFormat
	width                  uint32
	height                 uint32
	closed                 bool
}

// New creates a visible SDL window and a WebGPU surface for it.
func New(config platform.Config) (platform.Driver, error) {
	if err := platform.ValidateConfigFeatures(config); err != nil {
		return nil, err
	}

	if config.Width <= 0 {
		config.Width = 1280
	}

	if config.Height <= 0 {
		config.Height = 720
	}

	if config.Title == "" {
		config.Title = "WebGPU"
	}

	library := binsdl.Load()
	// Finger gestures are handled explicitly; generated mouse events would double
	// their movement and could leave a held primary button after cancellation.
	if err := sdl.SetHint(sdl.HINT_TOUCH_MOUSE_EVENTS, "0"); err != nil {
		library.Unload()

		return nil, fmt.Errorf("disable synthetic touch mouse events: %w", err)
	}

	if err := sdl.Init(sdl.INIT_VIDEO); err != nil {
		library.Unload()

		return nil, fmt.Errorf("initialize SDL: %w", err)
	}

	window, err := sdl.CreateWindow(
		config.Title,
		config.Width,
		config.Height,
		sdl.WINDOW_VULKAN|sdl.WINDOW_RESIZABLE|sdl.WINDOW_HIGH_PIXEL_DENSITY,
	)
	if err != nil {
		sdl.Quit()
		library.Unload()

		return nil, fmt.Errorf("create SDL window: %w", err)
	}

	driver := &Driver{window: window, sdlLibrary: library, sampleCount: config.SampleCount, metrics: config.Metrics}
	if err := initializeGPU(driver); err != nil {
		_ = driver.Close()

		return nil, err
	}

	if err := driver.prepareFeatures(config); err != nil {
		_ = driver.Close()

		return nil, err
	}

	if config.UI != nil {
		if _, err := config.UI.AttachUI(driver); err != nil {
			_ = driver.Close()

			return nil, err
		}
	}

	return driver, nil
}

func createSurface(instance *wgpu.Instance, window *sdl.Window) (*wgpu.Surface, error) {
	properties, err := window.Properties()
	if err != nil {
		return nil, fmt.Errorf("read SDL window properties: %w", err)
	}

	if properties.HasProperty(sdl.PROP_WINDOW_WAYLAND_DISPLAY_POINTER) && properties.HasProperty(sdl.PROP_WINDOW_WAYLAND_SURFACE_POINTER) {
		display := uintptr(properties.PointerProperty(sdl.PROP_WINDOW_WAYLAND_DISPLAY_POINTER))
		surface := uintptr(properties.PointerProperty(sdl.PROP_WINDOW_WAYLAND_SURFACE_POINTER))

		return instance.CreateSurfaceFromWaylandSurface(display, surface)
	}

	if properties.HasProperty(sdl.PROP_WINDOW_X11_DISPLAY_POINTER) && properties.HasProperty(sdl.PROP_WINDOW_X11_WINDOW_NUMBER) {
		display := uintptr(properties.PointerProperty(sdl.PROP_WINDOW_X11_DISPLAY_POINTER))

		windowNumber := properties.NumberProperty(sdl.PROP_WINDOW_X11_WINDOW_NUMBER, 0)
		if windowNumber <= 0 {
			return nil, errNoNativeWindow
		}

		windowID := uint64(windowNumber)

		return instance.CreateSurfaceFromXlibWindow(display, windowID)
	}

	return nil, errNoNativeWindow
}

func resize(driver *Driver) error {
	width, height, err := driver.window.SizeInPixels()
	if err != nil {
		return fmt.Errorf("read SDL pixel size: %w", err)
	}

	if width < 1 || height < 1 {
		return nil
	}

	driver.width, driver.height = uint32(width), uint32(height)

	if err := driver.surface.Configure(driver.device, &wgpu.SurfaceConfiguration{
		Format:      driver.format,
		Usage:       gputypes.TextureUsageRenderAttachment,
		Width:       driver.width,
		Height:      driver.height,
		AlphaMode:   gputypes.CompositeAlphaModeOpaque,
		PresentMode: gputypes.PresentModeFifo,
	}); err != nil {
		return err
	}

	return nil
}

// Run pumps native events and presents frames until the window closes or the
// context is canceled.
func (driver *Driver) Run(ctx context.Context, update func(time.Duration, platform.InputState) (render.Frame, error)) error {
	if driver.closed {
		return errDriverClosed
	}

	previous := time.Now()

	for {
		if driver.closed {
			return nil
		}

		if err := ctx.Err(); err != nil {
			return err
		}

		input, open, err := driver.readInput()
		if err != nil {
			return err
		}

		if !open {
			return nil
		}

		width, height, err := driver.window.SizeInPixels()
		if err != nil {
			return fmt.Errorf("read SDL pixel size: %w", err)
		}

		if width > 0 && height > 0 && (uint32(width) != driver.width || uint32(height) != driver.height) {
			if err := resize(driver); err != nil {
				return fmt.Errorf("resize WebGPU surface: %w", err)
			}
		}

		now := time.Now()
		input.Width = int(driver.width)
		input.Height = int(driver.height)
		driver.metrics.Begin(now.Sub(previous))
		frame, err := update(now.Sub(previous), input)
		previous = now

		if driver.closed {
			return nil
		}

		if err != nil {
			return fmt.Errorf("update frame: %w", err)
		}

		if err := presentFrame(driver, frame, width > 0 && height > 0); err != nil {
			return err
		}

		driver.metrics.Resources(len(driver.textures), driver.textureBytes)
		driver.metrics.Finish()
	}
}

// Uploads are independent of surface acquisition: a minimized/lost surface must
// not discard buffer writes that the App has already drained from its dirty queue.
func presentFrame(driver *Driver, frame render.Frame, renderable bool) error {
	if err := driver.uploadComputeBuffers(frame.Uploads); err != nil {
		return err
	}

	driver.metrics.Step(diagnostics.PreparePhase)

	if !renderable || driver.width == 0 || driver.height == 0 {
		if err := driver.submitComputeOnly(frame.Compute); err != nil {
			return err
		}

		// There is no presentation/VSync wait while minimized. Keep updates and
		// compute progressing without spinning the CPU or flooding the GPU queue.
		const minimizedFrameInterval = time.Second / 60

		time.Sleep(minimizedFrameInterval)

		return nil
	}

	return present(driver, frame)
}

func present(driver *Driver, frame render.Frame) error {
	defer driver.pruneMaterials()

	if err := driver.updateAttachedUI(frame.UI); err != nil {
		return err
	}

	if err := driver.updateSprites(frame.Screen2D); err != nil {
		return err
	}

	texture, _, err := driver.surface.GetCurrentTexture()
	if err != nil {
		if errors.Is(err, wgpu.ErrSurfaceNeedsReconfigure) || errors.Is(err, wgpu.ErrSurfaceLost) {
			if err := driver.submitComputeOnly(frame.Compute); err != nil {
				return err
			}

			return resize(driver)
		}

		return fmt.Errorf("acquire WebGPU surface texture: %w", err)
	}

	if texture == nil || texture.Texture == nil {
		return errEmptySurfaceFrame
	}
	defer texture.Texture.Release()

	view, err := texture.Texture.CreateView(nil)
	if err != nil {
		return fmt.Errorf("create surface texture view: %w", err)
	}
	defer view.Release()

	encoder, err := driver.device.CreateCommandEncoder(nil)
	if err != nil {
		return fmt.Errorf("create command encoder: %w", err)
	}
	defer encoder.Release()

	driver.metrics.Step(diagnostics.SurfacePhase)

	if err := driver.recordCompute(encoder, frame.Compute); err != nil {
		return err
	}

	driver.metrics.Step(diagnostics.ComputePhase)

	if err := driver.recordPointShadow(encoder, frame); err != nil {
		return err
	}

	if err := driver.drawScenePass(encoder, view, frame); err != nil {
		return err
	}

	if err := driver.drawSprites(encoder, view); err != nil {
		return err
	}

	driver.metrics.Step(diagnostics.RenderPhase)

	if driver.uiBridge != nil {
		if err := driver.uiBridge.Draw(encoder, view, frame.UI); err != nil {
			return err
		}
	}

	return submitAndPresent(driver, encoder, texture)
}

func submitAndPresent(driver *Driver, encoder *wgpu.CommandEncoder, texture *wgpu.SurfaceTexture) error {
	commands, err := encoder.Finish()
	if err != nil {
		return fmt.Errorf("finish command encoder: %w", err)
	}
	defer commands.Release()

	if _, err := driver.queue.Submit(commands); err != nil {
		return fmt.Errorf("submit frame: %w", err)
	}

	if err := driver.surface.Present(texture); err != nil {
		return fmt.Errorf("present frame: %w", err)
	}

	driver.metrics.Step(diagnostics.SubmitPhase)

	return nil
}

// Close releases GPU resources and closes the native window. It is safe to
// call more than once.
func (driver *Driver) Close() error {
	if driver.closed {
		return nil
	}

	driver.closed = true
	driver.closeSprites()

	if driver.uiBridge != nil {
		driver.uiBridge.Close()
		driver.uiBridge = nil
	}

	driver.releaseCompute()
	driver.releaseMaterials()

	if driver.shadowBridge != nil {
		driver.shadowBridge.Close()
		driver.shadowBridge = nil
	}

	if driver.pointShadow != nil {
		driver.pointShadow.release()
		driver.pointShadow = nil
	}

	driver.geometry.Clear(gpuGeometry.release)
	driver.releaseInstances()
	driver.releaseTargets()

	if driver.surface != nil {
		driver.surface.Release()
	}

	driver.releaseGraphicsPipelines()

	if driver.queue != nil {
		driver.queue.Release()
		driver.queue = nil
	}

	if driver.device != nil {
		driver.device.Release()
	}

	if driver.adapter != nil {
		driver.adapter.Release()
	}

	if driver.instance != nil {
		driver.instance.Release()
	}

	if driver.window != nil {
		driver.window.Destroy()
	}

	sdl.Quit()

	if driver.sdlLibrary != nil {
		driver.sdlLibrary.Unload()
		driver.sdlLibrary = nil
	}

	return nil
}
