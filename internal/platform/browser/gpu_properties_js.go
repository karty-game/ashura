//go:build js && wasm

package browser

const (
	triangleListTopology      = "triangle-list"
	topologyProperty          = "topology"
	addressModeUProperty      = "addressModeU"
	addressModeVProperty      = "addressModeV"
	magFilterProperty         = "magFilter"
	minFilterProperty         = "minFilter"
	mipmapFilterProperty      = "mipmapFilter"
	automaticLayout           = "auto"
	vertexProperty            = "vertex"
	fragmentProperty          = "fragment"
	targetsProperty           = "targets"
	cullModeProperty          = "cullMode"
	depthStencilProperty      = "depthStencil"
	depthCompareProperty      = "depthCompare"
	arrayLayerCountProperty   = "arrayLayerCount"
	cullModeNone              = "none"
	arrayStrideProperty       = "arrayStride"
	vertexEntry               = "vs_main"
	loadOpClear               = "clear"
	loadOpProperty            = "loadOp"
	storeOpProperty           = "storeOp"
	pointShadowTextureBinding = 2
	pointShadowTextureUsage   = 0x14
)

const (
	primitiveProperty         = "primitive"
	depthWriteEnabledProperty = "depthWriteEnabled"
	dimensionProperty         = "dimension"
	clampToEdge               = "clamp-to-edge"
	nearestFilter             = "nearest"
	stepModeProperty          = "stepMode"
	attributesProperty        = "attributes"
	viewProperty              = "view"
)
