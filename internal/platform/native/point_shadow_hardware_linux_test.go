//go:build linux && ashura_texture_native_review

package native

import (
	"math"
	"os"
	"strings"
	"testing"

	"github.com/gogpu/gputypes"
	"github.com/karty-game/ashura/internal/meshdata"
	"github.com/karty-game/ashura/internal/render"
	wgpu "github.com/karty-game/ashura/internal/webgpu"
)

const shadowReviewShader = `
@group(0) @binding(0) var image: texture_depth_2d_array;
@group(0) @binding(1) var<storage, read_write> result: array<f32>;
@compute @workgroup_size(1)
fn main(@builtin(global_invocation_id) id: vec3<u32>) {
 let center = vec2<i32>(textureDimensions(image)) / 2;
 result[id.x] = textureLoad(image, center, i32(id.x), 0);
}`

//nolint:paralleltest // Owns one hardware device and its error-scope stack.
func TestPointShadowFacesHardware(t *testing.T) {
	if os.Getenv("ASHURA_SHADOW_NATIVE_REVIEW") != "1" {
		t.Skip("requires explicit hardware Vulkan configuration")
	}

	driver := openCompressionReviewDevice(t)
	driver.sampleCount, driver.format = 1, gputypes.TextureFormatRGBA8Unorm
	driver.device.PushErrorScope(wgpu.ErrorFilterValidation)
	t.Cleanup(func() {
		kind, message, err := driver.device.PopErrorScopeAsync(driver.instance)
		if err != nil || kind != wgpu.ErrorTypeNoError {
			t.Error("GPU validation", kind, message, err)
		}
	})

	if err := RegisterStandard(driver); err != nil {
		t.Fatal(err)
	}

	t.Cleanup(driver.releaseGraphicsPipelines)

	if err := driver.preparePointShadowSampling(64); err != nil {
		t.Fatal(err)
	}

	t.Cleanup(driver.pointShadow.release)

	if err := RegisterPointShadows(driver, 64); err != nil {
		t.Fatal(err)
	}

	t.Cleanup(driver.shadowBridge.Close)
	t.Cleanup(func() { driver.geometry.Clear(gpuGeometry.release) })
	t.Cleanup(driver.releaseInstances)

	frame := shadowReviewFrame()
	renderShadowReview(t, driver, frame)
	checkShadowReviewDepth(t, sampleShadowReview(t, driver), [6]float32{.2, .2, .2, .2, .2, .2})

	// No encoder is available: unchanged cached depth must not begin any pass.
	if err := driver.recordPointShadow(nil, frame); err != nil {
		t.Fatal("unchanged shadow attempted recording", err)
	}

	frame.Draws[0].Transform[12] = .5
	renderShadowReview(t, driver, frame)
	checkShadowReviewDepth(t, sampleShadowReview(t, driver), [6]float32{.25, .15, .2, .2, .2, .2})

	frame.Lights[0].Direction[0] = .25
	renderShadowReview(t, driver, frame)
	checkShadowReviewDepth(t, sampleShadowReview(t, driver), [6]float32{.225, .175, .2, .2, .2, .2})

	frame.Lights[0].Direction[0] = 0
	frame.Draws[0].Transform = [16]float32{0: .25, 5: .25, 10: .25, 12: 5, 15: 1}
	renderShadowReview(t, driver, frame)
	checkShadowReviewDepth(t, sampleShadowReview(t, driver), [6]float32{.45, 1, 1, 1, 1, 1})

	frame.Draws[0].Transform[12] = 20
	renderShadowReview(t, driver, frame)
	checkShadowReviewDepth(t, sampleShadowReview(t, driver), [6]float32{1, 1, 1, 1, 1, 1})
	t.Log("six projected faces, cached reuse, caster and light movement, face and range culling passed")
}

func shadowReviewFrame() render.Frame {
	vertices := make([]render.Vertex, 8)
	for index := range vertices {
		vertices[index].Position = [3]float32{-2, -2, -2}
		for axis := range 3 {
			if index&(1<<axis) != 0 {
				vertices[index].Position[axis] = 2
			}
		}
	}

	return render.Frame{
		Draws: []render.DrawCall{{ID: 1, Vertices: vertices,
			Bounds: meshdata.Bounds{Minimum: [3]float32{-2, -2, -2}, Maximum: [3]float32{2, 2, 2}, Valid: true},
			Indices: []uint32{0, 4, 6, 0, 6, 2, 1, 3, 7, 1, 7, 5, 0, 1, 5, 0, 5, 4,
				2, 6, 7, 2, 7, 3, 0, 2, 3, 0, 3, 1, 4, 5, 7, 4, 7, 6},
			Transform: [16]float32{0: 1, 5: 1, 10: 1, 15: 1},
		}},
		Lights: []render.Light{{Kind: render.PointLightKind, Intensity: 1, Color: [3]float32{1, 1, 1},
			GroundColor: [3]float32{10}, CastShadows: true, ShadowResolution: 64,
		}},
	}
}

func renderShadowReview(t *testing.T, driver *Driver, frame render.Frame) {
	t.Helper()

	encoder, err := driver.device.CreateCommandEncoder(nil)
	if err != nil {
		t.Fatal(err)
	}
	defer encoder.Release()

	// Invalid descriptors must fail at the binding boundary, without native calls.
	if _, err := encoder.BeginRenderPass(&wgpu.RenderPassDescriptor{}); err == nil || !strings.Contains(err.Error(), "no color or depth") {
		t.Fatal("empty render pass was admitted", err)
	}

	missingDepth := &wgpu.RenderPassDescriptor{DepthStencilAttachment: &wgpu.RenderPassDepthStencilAttachment{}}
	if _, err := encoder.BeginRenderPass(missingDepth); err == nil {
		t.Fatal("missing depth view was admitted")
	}

	if err := driver.geometry.Prepare(frame.Draws); err != nil {
		t.Fatal(err)
	}

	if err := driver.recordPointShadow(encoder, frame); err != nil {
		t.Fatal(err)
	}

	submitShadowReview(t, driver, encoder)
}

func submitShadowReview(t *testing.T, driver *Driver, encoder *wgpu.CommandEncoder) {
	t.Helper()

	commands, err := encoder.Finish()
	if err != nil {
		t.Fatal(err)
	}
	defer commands.Release()

	if _, err := driver.queue.Submit(commands); err != nil {
		t.Fatal(err)
	}
}

func checkShadowReviewDepth(t *testing.T, values []float32, want [6]float32) {
	t.Helper()

	if len(values) != len(want) {
		t.Fatal("incorrect shadow readback length", len(values))
	}

	t.Log("six-face projected depth:", values)

	// Convert projected depth back to the fixture's original distance
	// fraction, preserving its world-space error tolerance.
	const nearFraction = float32(.002)

	for face, expected := range want {
		depth := values[face]
		distance := nearFraction / (1 - depth*(1-nearFraction))
		if math.IsNaN(float64(depth)) || math.Abs(float64(distance-expected)) > .001 {
			t.Fatalf("face %d depth %v corresponds to %v, want distance fraction %v", face, depth, distance, expected)
		}
	}
}

func sampleShadowReview(t *testing.T, driver *Driver) []float32 {
	t.Helper()

	view, err := driver.pointShadow.image.CreateView(&wgpu.TextureViewDescriptor{
		Dimension: gputypes.TextureViewDimension2DArray, MipLevelCount: 1, ArrayLayerCount: render.PointShadowFaces,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer view.Release()

	shader, err := driver.device.CreateShaderModuleWGSL(shadowReviewShader)
	if err != nil {
		t.Fatal(err)
	}
	defer shader.Release()

	pipeline, err := driver.device.CreateComputePipeline(&wgpu.ComputePipelineDescriptor{Module: shader, EntryPoint: "main"})
	if err != nil {
		t.Fatal(err)
	}
	defer pipeline.Release()

	layout := pipeline.GetBindGroupLayout(0)
	defer layout.Release()

	const size = render.PointShadowFaces * 4

	output, err := driver.device.CreateBuffer(&wgpu.BufferDescriptor{
		Size: size, Usage: gputypes.BufferUsageStorage | gputypes.BufferUsageCopySrc,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer output.Release()

	readback, err := driver.device.CreateBuffer(&wgpu.BufferDescriptor{
		Size: size, Usage: gputypes.BufferUsageMapRead | gputypes.BufferUsageCopyDst,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer readback.Release()

	group, err := driver.device.CreateBindGroup(&wgpu.BindGroupDescriptor{Layout: layout, Entries: []wgpu.BindGroupEntry{
		{Binding: 0, TextureView: view}, {Binding: 1, Buffer: output, Size: size},
	}})
	if err != nil {
		t.Fatal(err)
	}
	defer group.Release()

	encoder, err := driver.device.CreateCommandEncoder(nil)
	if err != nil {
		t.Fatal(err)
	}
	defer encoder.Release()

	pass, err := encoder.BeginComputePass(nil)
	if err != nil {
		t.Fatal(err)
	}

	pass.SetPipeline(pipeline)
	pass.SetBindGroup(0, group, nil)
	pass.DispatchWorkgroups(render.PointShadowFaces, 1, 1)
	pass.End()
	pass.Release()
	encoder.CopyBufferToBuffer(output, 0, readback, 0, size)
	submitShadowReview(t, driver, encoder)

	return readReviewSamples(t, driver, readback, size)
}
