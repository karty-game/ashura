//go:build js && wasm

package browser

import (
	"github.com/karty-game/karty-webgpu/internal/spritepaint"
)

const (
	spriteVertices     = 6
	spriteStorageUsage = 0xa8 // STORAGE | COPY_DST | VERTEX.
	spriteUniformUsage = 0x48 // UNIFORM | COPY_DST.
	spriteViewportSize = 16
)

func (driver *spriteFeature) prepareSprites() error {
	if driver.sprites != nil {
		return nil
	}

	finishScope := driver.beginSpriteResources()
	defer finishScope()

	module := driver.device.Call("createShaderModule", jsObject(map[string]any{
		labelProperty: "Screen2D sprites", "code": spritepaint.Shader,
	}))
	blend := jsObject(map[string]any{"operation": "add", "srcFactor": "one", "dstFactor": "one-minus-src-alpha"})
	descriptor := jsObject(map[string]any{
		labelProperty: "Screen2D pixel sprites", layoutProperty: "auto",
		"vertex": jsObject(map[string]any{
			moduleProperty: module, entryPointProperty: "vertexMain", uiVertexBuffers: spriteVertexLayout(),
		}),
		"fragment": jsObject(map[string]any{
			moduleProperty: module, entryPointProperty: "fragmentMain",
			"targets": []any{jsObject(map[string]any{
				formatProperty: driver.colorFormat, "blend": jsObject(map[string]any{"color": blend, "alpha": blend}),
			})},
		}),
		"primitive": jsObject(map[string]any{"topology": "triangle-list", "cullMode": "none"}),
	})

	pipeline, err := awaitPromise(driver.device.Call("createRenderPipelineAsync", descriptor))
	if err != nil {
		return err
	}

	gpu := &spriteGPU{pipeline: pipeline, pages: make(map[*spritepaint.Texture]uiPage)}
	gpu.passAttachment = jsObject(map[string]any{loadOpProperty: "load", storeOpProperty: uiStore})
	gpu.passDescriptor = jsObject(map[string]any{uiAttachments: []any{gpu.passAttachment}})
	gpu.uniform = driver.device.Call("createBuffer", jsObject(map[string]any{
		labelProperty: "Screen2D viewport", sizeProperty: spriteViewportSize, usageProperty: spriteUniformUsage,
	}))
	gpu.sampler = driver.device.Call("createSampler", jsObject(map[string]any{
		"magFilter": nearestFilter, "minFilter": nearestFilter, "mipmapFilter": nearestFilter,
		"addressModeU": clampToEdge, "addressModeV": clampToEdge,
	}))
	driver.sprites = gpu

	return nil
}

func spriteVertexLayout() []any {
	const (
		vectorBytes = 16
		uvLocation  = 2
	)

	return []any{jsObject(map[string]any{
		"arrayStride": spritepaint.Stride, "stepMode": uiInstanceStep,
		"attributes": []any{
			jsObject(map[string]any{formatProperty: uiFloat4Format, offsetProperty: 0, shaderLocationProperty: 0}),
			jsObject(map[string]any{formatProperty: uiFloat4Format, offsetProperty: vectorBytes, shaderLocationProperty: 1}),
			jsObject(
				map[string]any{
					formatProperty:         uiFloat4Format,
					offsetProperty:         uvLocation * vectorBytes,
					shaderLocationProperty: uvLocation,
				},
			),
		},
	})}
}
