//go:build js && wasm

package browser

import "errors"

var (
	errTextureUpload      = errors.New("browser: texture upload failed")
	errBrowserEnvironment = errors.New("browser platform requires window, document, and navigator")
	errGPUUnavailable     = errors.New("WebGPU is not available in this browser")
	errCanvasContext      = errors.New("canvas does not provide a WebGPU context")
	errNoAdapter          = errors.New("browser did not provide a WebGPU adapter")
	errCanvasFormat       = errors.New("browser returned an empty preferred canvas format")
	errDocumentBody       = errors.New("browser document has no body for the WebGPU canvas")
	errDriverClosed       = errors.New("browser driver is closed")
	errDriverRunning      = errors.New("browser driver is already running")
	errSurfaceTexture     = errors.New("WebGPU returned no current canvas texture")
	errSurfaceView        = errors.New("create canvas texture view returned no view")
	errCommandEncoder     = errors.New("create command encoder returned no encoder")
	errRenderPass         = errors.New("begin render pass returned no pass encoder")
	errCommandBuffer      = errors.New("finish command encoder returned no command buffer")
	errRenderPipeline     = errors.New("create default WebGPU pipeline returned no pipeline")
	errDepthTexture       = errors.New("create depth texture returned no texture")
	errDepthView          = errors.New("create depth texture view returned no view")
	errDrawBuffer         = errors.New("create WebGPU draw buffer returned no buffer")
	errCaptureUnsupported = errors.New("browser WebGPU capture is not implemented")
	errPromiseRejected    = errors.New("JavaScript promise rejected")
)

const (
	formatProperty     = "format"
	labelProperty      = "label"
	entriesProperty    = "entries"
	layoutProperty     = "layout"
	entryPointProperty = "entryPoint"
	moduleProperty     = "module"
	formatFloat32x3    = "float32x3"
)

const (
	linearFilter               = "linear"
	offsetProperty             = "offset"
	shaderLocationProperty     = "shaderLocation"
	sizeProperty               = "size"
	widthProperty              = "width"
	heightProperty             = "height"
	depthOrArrayLayersProperty = "depthOrArrayLayers"
	usageProperty              = "usage"
	bindingProperty            = "binding"
	resourceProperty           = "resource"
)
