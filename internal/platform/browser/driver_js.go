//go:build js && wasm

// Package browser implements the WebGPU browser platform driver.
package browser

import (
	"context"
	"fmt"
	"image"
	"sync"
	"syscall/js"
	"time"

	"github.com/karty-game/karty-webgpu/internal/diagnostics"
	"github.com/karty-game/karty-webgpu/internal/impostordata"
	"github.com/karty-game/karty-webgpu/internal/platform"
	"github.com/karty-game/karty-webgpu/internal/render"
	"github.com/karty-game/karty-webgpu/internal/texturedata"
)

type inputListener struct {
	target js.Value
	event  string
	fn     js.Func
}

// Driver owns the browser canvas and WebGPU presentation objects.
type browserDraw struct {
	call      render.DrawCall
	count     int
	geometry  gpuGeometry
	instances int
	instance  *gpuInstances
}

// Driver owns browser presentation and cached material resources.
type Driver struct {
	uiBridge                uiBackend
	spriteBridge            spriteBackend
	shadowBridge            shadowBackend
	computeBridge           computeBackendInterface
	pointShadow             *gpuPointShadow
	queue                   js.Value
	metrics                 *diagnostics.Monitor
	textureBytes            uint64
	sampleCount             uint32
	colorFormat             string
	colorTexture, colorView js.Value
	draws                   []browserDraw
	geometry                render.GeometryCache[gpuGeometry]
	instances               map[uint32]*gpuInstances
	submitBuffers           js.Value
	textures                map[*texturedata.Texture]gpuTexture
	materials               map[uint32]*gpuMaterial
	impostorViews           map[*impostordata.Document]*gpuImpostor
	impostorSampler         js.Value
	white, flat, whiteAO    *texturedata.Texture
	sampler                 js.Value
	lightmapSampler         js.Value

	canvas                 js.Value
	window                 js.Value
	indexFormat            js.Value
	storeOperation         js.Value
	discardOperation       js.Value
	context                js.Value
	device                 js.Value
	standardPipeline       js.Value
	standardCulledPipeline js.Value
	sourceCulledPipeline   js.Value
	sourcePipeline         js.Value
	impostorPipeline       js.Value
	clearValue             js.Value
	colorAttach            js.Value
	depthAttach            js.Value
	passDescriptor         js.Value
	attachments            js.Value
	depthTexture           js.Value
	depthView              js.Value
	depthWidth             int
	depthHeight            int
	canvasWidth            int
	canvasHeight           int
	touch                  platform.TouchTracker
	pointerLockAllowed     bool
	lastPointerTouch       bool
	input                  platform.InputState
	listeners              []inputListener

	deviceWatchDone chan struct{}
	deviceErr       error
	mu              sync.Mutex
	closed          bool
	running         bool
	frame           js.Func
	frameID         js.Value
	done            chan struct{}
	runErr          error
	finishOnce      sync.Once
	lastFrameAt     time.Time
}

// New obtains a browser WebGPU device and selects or creates a canvas.
func New(config platform.Config) (platform.Driver, error) {
	if err := platform.ValidateConfigFeatures(config); err != nil {
		return nil, err
	}

	window, document, gpu, err := browserEnvironment()
	if err != nil {
		return nil, err
	}

	canvas, err := getCanvas(document, config)
	if err != nil {
		return nil, err
	}

	canvas.Set("width", config.Width)
	canvas.Set("height", config.Height)
	// Preserve host CSS; attributes supply the intrinsic size.

	context := canvas.Call("getContext", "webgpu")
	if context.IsUndefined() || context.IsNull() {
		return nil, errCanvasContext
	}

	adapter, err := awaitPromise(gpu.Call("requestAdapter"))
	if err != nil {
		return nil, fmt.Errorf("request WebGPU adapter: %w", err)
	}

	if adapter.IsNull() || adapter.IsUndefined() {
		return nil, errNoAdapter
	}

	device, err := awaitPromise(adapter.Call("requestDevice", jsObject(map[string]any{"requiredFeatures": compressionFeatures(adapter)})))
	if err != nil {
		return nil, fmt.Errorf("request WebGPU device: %w", err)
	}

	format := gpu.Call("getPreferredCanvasFormat").String()
	if format == "" {
		device.Call("destroy")

		return nil, errCanvasFormat
	}

	configureSurface(context, device, format)

	driver := &Driver{
		window: window,
		canvas: canvas, context: context, device: device, queue: device.Get("queue"),
		sampleCount: config.SampleCount, colorFormat: format, metrics: config.Metrics,
	}
	if err := driver.prepareScene(config); err != nil {
		return nil, err
	}

	if err := driver.prepareInput(config); err != nil {
		_ = driver.Close()

		return nil, err
	}

	driver.deviceWatchDone = make(chan struct{})
	go driver.watchDevice()

	return driver, nil
}

func browserEnvironment() (window, document, gpu js.Value, err error) {
	window, document = js.Global().Get("window"), js.Global().Get("document")

	navigator := js.Global().Get("navigator")
	if window.IsUndefined() || document.IsUndefined() || navigator.IsUndefined() {
		return window, document, gpu, errBrowserEnvironment
	}

	gpu = navigator.Get("gpu")
	if gpu.IsUndefined() || gpu.IsNull() {
		return window, document, gpu, errGPUUnavailable
	}

	return window, document, gpu, nil
}

func getCanvas(document js.Value, config platform.Config) (js.Value, error) {
	var canvas js.Value
	if config.CanvasID != "" {
		canvas = document.Call("getElementById", config.CanvasID)
	}

	if canvas.IsUndefined() || canvas.IsNull() {
		canvas = document.Call("createElement", "canvas")
		if config.CanvasID != "" {
			canvas.Set("id", config.CanvasID)
		}

		body := document.Get("body")
		if body.IsUndefined() || body.IsNull() {
			return js.Undefined(), errDocumentBody
		}

		body.Call("appendChild", canvas)
	}

	return canvas, nil
}

// Capture will provide the explicit asynchronous readback API in a later slice.
func (driver *Driver) Capture(ctx context.Context) (*image.RGBA, error) {
	_ = ctx

	return nil, errCaptureUnsupported
}

// Close cancels the active animation callback and releases the WebGPU device.
func (driver *Driver) Close() error {
	driver.mu.Lock()
	if driver.closed {
		driver.mu.Unlock()

		return nil
	}

	driver.closed = true
	driver.closeSprites()

	if driver.uiBridge != nil {
		driver.uiBridge.Close()
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

	running := driver.running
	driver.mu.Unlock()
	driver.removeInputListeners()

	if js.Global().Get("document").Get("pointerLockElement").Equal(driver.canvas) {
		js.Global().Get("document").Call("exitPointerLock")
	}

	if running {
		driver.finish(nil)
		driver.cancelFrame()
	}

	driver.geometry.Clear(gpuGeometry.release)

	driver.releaseInstances()

	driver.releaseTargets()

	if !driver.device.IsUndefined() && !driver.device.IsNull() {
		driver.device.Call("destroy")
	}
	// Close can run inside an animation callback. The Run waiter drains the lost
	// promise after that callback returns; setup/error cleanup can drain it here.
	if !running && driver.deviceWatchDone != nil {
		<-driver.deviceWatchDone
	}

	return nil
}

func (driver *Driver) resizeCanvas() {
	width := driver.canvas.Get("clientWidth").Int()

	height := driver.canvas.Get("clientHeight").Int()
	if width <= 0 || height <= 0 {
		return
	}

	ratio := driver.window.Get("devicePixelRatio")

	dpr := 1.0
	if ratio.Type() == js.TypeNumber && ratio.Float() > 0 {
		dpr = ratio.Float()
	}

	pixelWidth := int(float64(width) * dpr)
	pixelHeight := int(float64(height) * dpr)

	if driver.canvas.Get("width").Int() != pixelWidth {
		driver.canvas.Set("width", pixelWidth)
	}

	if driver.canvas.Get("height").Int() != pixelHeight {
		driver.canvas.Set("height", pixelHeight)
	}

	driver.canvasWidth, driver.canvasHeight = pixelWidth, pixelHeight
	driver.input.LogicalHeight = float32(height)

	driver.input.LogicalWidth = float32(width)
	if driver.uiBridge != nil {
		driver.uiBridge.resize(float32(width), float32(height))
	}
}

func configureSurface(context, device js.Value, format string) {
	context.Call("configure", jsObject(map[string]any{
		"device":       device,
		formatProperty: format,
		"alphaMode":    "opaque",
	}))
}

func (driver *Driver) prepareScene(config platform.Config) error {
	if err := driver.prepareScenePass(); err != nil {
		driver.device.Call("destroy")

		return err
	}

	featureConfig := platform.FeatureConfig{PointShadowResolution: config.PointShadowResolution}
	if err := platform.InstallMaterialFeatures(driver, config.Features, featureConfig); err != nil {
		_ = driver.Close()

		return err
	}

	if err := driver.preparePointShadowSampling(config.PointShadowResolution); err != nil {
		_ = driver.Close()

		return err
	}

	if err := platform.InstallOptionalFeatures(driver, config.Features, featureConfig); err != nil {
		_ = driver.Close()

		return err
	}

	return nil
}

func (driver *Driver) prepareInput(config platform.Config) error {
	driver.installInput()

	if config.UI != nil {
		_, err := config.UI.AttachUI(driver)

		return err
	}

	return nil
}
