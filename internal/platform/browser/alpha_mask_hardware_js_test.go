//go:build js && wasm

package browser

import (
	"math"
	"syscall/js"
	"testing"

	"github.com/karty-game/karty-webgpu/internal/render"
)

//nolint:paralleltest // One hardware device owns sequential material mutations and pixel readbacks.
func TestStandardAlphaMaskHardwareCoverage(t *testing.T) {
	for _, format := range []string{"rgba8unorm", "rgba8unorm-srgb"} {
		t.Run(format, func(t *testing.T) {
			driver := spriteHardwareDriver(t, format)
			pipeline, uniform, group, shadow := alphaMaskMaterialFixture(t, driver, format)

			for _, test := range []struct {
				name                     string
				masked                   bool
				cutoff, tint             float32
				left, low, middle, right bool
			}{
				{"opaque ignores transparent texel", false, .5, 1, true, true, true, true},
				{"mask follows texture alpha", true, .5, 1, false, false, true, true},
				{"mask multiplies tint alpha", true, .5, .25, false, false, false, false},
				{"zero cutoff and tint preserve equality", true, 0, 0, true, true, true, true},
				{"unit cutoff preserves equality", true, 1, 1, false, false, false, true},
				{"half cutoff preserves equality", true, .5, .5, false, false, false, true},
				{"opaque ignores zero tint alpha", false, .5, 0, true, true, true, true},
				{"one-byte cutoff equality", true, float32(1) / 255, 1, false, true, true, true},
				{"one-byte cutoff below", true, math.Nextafter32(float32(1)/255, 0), 1, false, true, true, true},
				{"one-byte cutoff above", true, math.Nextafter32(float32(1)/255, 1), 1, false, false, true, true},
				{"middle-byte cutoff equality", true, float32(128) / 255, 1, false, false, true, true},
				{"middle-byte cutoff above", true, math.Nextafter32(float32(128)/255, 1), 1, false, false, false, true},
			} {
				data := render.Uniforms(render.DrawCall{AlphaMask: test.masked, AlphaCutoff: test.cutoff,
					Color: [4]float32{1, 1, 1, test.tint}}, render.Frame{Lights: []render.Light{{
					Kind: render.HemisphereLightKind, Direction: [3]float32{0, 1, 0},
					Color: [3]float32{1, 1, 1}, GroundColor: [3]float32{1, 1, 1}, Intensity: 1,
				}}})
				array := js.Global().Get("Uint8Array").New(len(data))
				js.CopyBytesToJS(array, data[:])
				driver.queue.Call("writeBuffer", uniform, 0, array)
				actual := alphaMaskCoveragePixels(t, driver, pipeline, group, shadow)

				for _, pixel := range []struct {
					column  int
					visible bool
				}{{1, test.left}, {2, test.low}, {4, test.middle}, {6, test.right}} {
					offset := (2*8 + pixel.column) * 4

					visible := actual[offset] != 0 || actual[offset+1] != 0 || actual[offset+2] != 0
					if visible != pixel.visible || actual[offset+3] != 255 {
						t.Fatal(test.name, "coverage mismatch", pixel.column, actual[offset:offset+4], pixel.visible)
					}
				}
			}

			if err := driver.deviceFailure(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func alphaMaskMaterialFixture(t *testing.T, driver *Driver, format string) (js.Value, js.Value, js.Value, js.Value) {
	t.Helper()

	device := driver.device
	shader := device.Call("createShaderModule", jsObject(map[string]any{"code": render.StandardShader() + `
@vertex fn alphaTestVertex(@builtin(vertex_index) index: u32) -> VertexOutput {
    let positions = array<vec2f, 3>(vec2f(-1.0, -1.0), vec2f(3.0, -1.0), vec2f(-1.0, 3.0));
    let position = positions[index];
    var output: VertexOutput;
    output.position = vec4f(position, 0.5, 1.0);
    output.worldPosition = vec3f(position, 0.0);
    output.normal = vec3f(0.0, 0.0, 1.0);
    output.uv = position * 0.5 + vec2f(0.5);
    output.lightmapUV = output.uv;
    return output;
}`}))
	pipeline := device.Call("createRenderPipeline", jsObject(map[string]any{
		"layout": "auto",
		"vertex": jsObject(map[string]any{"module": shader, "entryPoint": "alphaTestVertex"}),
		"fragment": jsObject(map[string]any{"module": shader, "entryPoint": render.FragmentEntry(format == "rgba8unorm"),
			"targets": []any{jsObject(map[string]any{"format": format})}}),
	}))
	uniform := device.Call("createBuffer", jsObject(map[string]any{"size": render.StandardUniformSize, "usage": 72}))

	t.Cleanup(func() { uniform.Call("destroy") })

	albedo := device.Call("createTexture", jsObject(map[string]any{"size": []any{4, 1, 1}, "format": "rgba8unorm-srgb", "usage": 6}))

	t.Cleanup(func() { albedo.Call("destroy") })

	pixels := js.Global().Get("Uint8Array").New(16)
	js.CopyBytesToJS(pixels, []byte{255, 255, 255, 0, 255, 255, 255, 1, 255, 255, 255, 128, 255, 255, 255, 255})
	driver.queue.Call("writeTexture", jsObject(map[string]any{"texture": albedo}), pixels,
		jsObject(map[string]any{"bytesPerRow": 16}), []any{4, 1, 1})
	sampler := device.Call("createSampler")
	entries := make([]any, 0, 8)

	entries = append(entries,
		jsObject(map[string]any{"binding": 0, "resource": jsObject(map[string]any{"buffer": uniform})}),
		jsObject(map[string]any{"binding": 1, "resource": sampler}),
	)
	for _, binding := range []int{2, 3, 4, 5, 6} {
		entries = append(entries, jsObject(map[string]any{"binding": binding, "resource": albedo.Call("createView")}))
	}

	entries = append(entries, jsObject(map[string]any{"binding": 7, "resource": sampler}))
	group := device.Call("createBindGroup", jsObject(map[string]any{"layout": pipeline.Call("getBindGroupLayout", 0), "entries": entries}))
	shadow := alphaMaskShadowFixture(t, driver, pipeline)

	return pipeline, uniform, group, shadow
}

func alphaMaskShadowFixture(t *testing.T, driver *Driver, pipeline js.Value) js.Value {
	t.Helper()

	device := driver.device
	uniform := device.Call("createBuffer", jsObject(map[string]any{"size": render.PointShadowSampleSize, "usage": 64}))
	texture := device.Call("createTexture", jsObject(map[string]any{
		"size": []any{1, 1, 6}, "format": "depth32float", "textureBindingViewDimension": "cube", "usage": 20,
	}))

	t.Cleanup(func() { uniform.Call("destroy"); texture.Call("destroy") })

	return device.Call("createBindGroup", jsObject(map[string]any{
		"layout": pipeline.Call("getBindGroupLayout", 1), "entries": []any{
			jsObject(map[string]any{"binding": 0, "resource": jsObject(map[string]any{"buffer": uniform})}),
			jsObject(
				map[string]any{"binding": 1, "resource": device.Call("createSampler", jsObject(map[string]any{"compare": "less-equal"}))},
			),
			jsObject(map[string]any{"binding": 2, "resource": texture.Call("createView", jsObject(map[string]any{"dimension": "cube"}))}),
		},
	}))
}

func alphaMaskCoveragePixels(t *testing.T, driver *Driver, pipeline, material, shadow js.Value) []byte {
	t.Helper()

	device := driver.device

	target := device.Call("createTexture", jsObject(map[string]any{"size": []any{8, 4, 1}, "format": driver.colorFormat, "usage": 17}))
	defer target.Call("destroy")

	readback := device.Call("createBuffer", jsObject(map[string]any{"size": 256 * 4, "usage": 9}))
	defer readback.Call("destroy")

	encoder := device.Call("createCommandEncoder")
	pass := encoder.Call("beginRenderPass", jsObject(map[string]any{"colorAttachments": []any{jsObject(map[string]any{
		"view": target.Call("createView"), "loadOp": "clear", "storeOp": "store", "clearValue": []any{0, 0, 0, 1},
	})}}))
	pass.Call("setPipeline", pipeline)
	pass.Call("setBindGroup", 0, material)
	pass.Call("setBindGroup", 1, shadow)
	pass.Call("draw", 3)
	pass.Call("end")
	encoder.Call("copyTextureToBuffer", jsObject(map[string]any{"texture": target}),
		jsObject(map[string]any{"buffer": readback, "bytesPerRow": 256}), []any{8, 4, 1})
	driver.queue.Call("submit", []any{encoder.Call("finish")})

	if _, err := awaitPromise(readback.Call("mapAsync", 1)); err != nil {
		t.Fatal(err)
	}

	data := make([]byte, 256*4)
	js.CopyBytesToGo(data, js.Global().Get("Uint8Array").New(readback.Call("getMappedRange")))
	readback.Call("unmap")

	pixels := make([]byte, 8*4*4)
	for row := range 4 {
		copy(pixels[row*8*4:(row+1)*8*4], data[row*256:row*256+8*4])
	}

	return pixels
}
