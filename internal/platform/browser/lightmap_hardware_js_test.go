//go:build js && wasm

package browser

import (
	"encoding/binary"
	"math"
	"syscall/js"
	"testing"

	"github.com/karty-game/karty-webgpu/internal/render"
)

//nolint:paralleltest // The hardware fixture owns one device and checks its complete upload/dispatch/readback boundary.
func TestCalibratedRGBEFiltersDecodedEnergy(t *testing.T) {
	driver := spriteHardwareDriver(t, "rgba8unorm")
	device := driver.device

	image := device.Call("createTexture", jsObject(map[string]any{
		"size": []any{2, 1, 1}, "format": "rgba8unorm", "usage": 6,
	}))
	defer image.Call("destroy")

	pixels := js.Global().Get("Uint8Array").New(8)
	js.CopyBytesToJS(pixels, []byte{255, 0, 0, 128, 0, 255, 0, 130})
	driver.queue.Call("writeTexture", jsObject(map[string]any{"texture": image}), pixels,
		jsObject(map[string]any{"bytesPerRow": 8}), []any{2, 1, 1})

	output := device.Call("createBuffer", jsObject(map[string]any{"size": 48, "usage": 132}))
	defer output.Call("destroy")

	readback := device.Call("createBuffer", jsObject(map[string]any{"size": 48, "usage": 9}))
	defer readback.Call("destroy")

	shader := device.Call("createShaderModule", jsObject(map[string]any{"code": render.StandardShader() + `
@group(0) @binding(8) var<storage, read_write> testEnergy: array<vec4f>;
@compute @workgroup_size(1) fn testRGBE() {
    testEnergy[0] = vec4f(sampleDiffuseRGBE(vec2f(0.5)), 1.0);
    testEnergy[1] = vec4f(sampleDiffuseRGBE(vec2f(0.0, 0.5)), 1.0);
    testEnergy[2] = vec4f(sampleDiffuseRGBE(vec2f(1.0, 0.5)), 1.0);
}`}))
	pipeline := device.Call("createComputePipeline", jsObject(map[string]any{
		"layout": "auto", "compute": jsObject(map[string]any{"module": shader, "entryPoint": "testRGBE"}),
	}))
	group := device.Call("createBindGroup", jsObject(map[string]any{
		"layout": pipeline.Call("getBindGroupLayout", 0), "entries": []any{
			jsObject(map[string]any{"binding": 6, "resource": image.Call("createView")}),
			jsObject(map[string]any{"binding": 8, "resource": jsObject(map[string]any{"buffer": output})}),
		},
	}))
	encoder := device.Call("createCommandEncoder")
	pass := encoder.Call("beginComputePass")
	pass.Call("setPipeline", pipeline)
	pass.Call("setBindGroup", 0, group)
	pass.Call("dispatchWorkgroups", 1)
	pass.Call("end")
	encoder.Call("copyBufferToBuffer", output, 0, readback, 0, 48)
	driver.queue.Call("submit", []any{encoder.Call("finish")})

	if _, err := awaitPromise(readback.Call("mapAsync", 1)); err != nil {
		t.Fatal(err)
	}

	data := make([]byte, 48)
	js.CopyBytesToGo(data, js.Global().Get("Uint8Array").New(readback.Call("getMappedRange")))
	readback.Call("unmap")

	for index, expected := range []float32{.5, 2, 0, 1, 1, 0, 0, 1, 0, 4, 0, 1} {
		actual := math.Float32frombits(binary.LittleEndian.Uint32(data[index*4:]))
		if math.Abs(float64(actual-expected)) > 1e-5 {
			t.Fatalf("decoded lightmap channel %d: got %g want %g", index, actual, expected)
		}
	}

	if err := driver.deviceFailure(); err != nil {
		t.Fatal(err)
	}
}
