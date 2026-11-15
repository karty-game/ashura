//go:build linux && ashura_texture_native_review

package native

import (
	"encoding/binary"
	"math"
	"os"
	"strings"
	"testing"
	"time"
	"unsafe"

	"github.com/gogpu/gputypes"
	"github.com/karty-game/karty-webgpu/internal/ktx2data"
	"github.com/karty-game/karty-webgpu/internal/texturedata"
	wgpu "github.com/karty-game/karty-webgpu/internal/webgpu"
)

// This shader reads both ends of every logical mip, including sub-block tails.
// textureLoad uses explicit mip levels, independent of sampler LOD defaults.
const compressionReviewShader = `
@group(0) @binding(0) var image: texture_2d<f32>;
@group(0) @binding(1) var<storage, read_write> result: array<vec4<f32>>;
@compute @workgroup_size(1)
fn main(@builtin(global_invocation_id) id: vec3<u32>) {
    let mip = i32(id.x);
    let edge = vec2<i32>(textureDimensions(image, mip)) - vec2<i32>(1);
    result[id.x * 2u] = textureLoad(image, vec2<i32>(0), mip);
    result[id.x * 2u + 1u] = textureLoad(image, edge, mip);
}`

//nolint:paralleltest // Owns one hardware device and its error-scope stack.
func TestPreparedCompressedTexturesHardware(t *testing.T) {
	if os.Getenv("ASHURA_TEXTURE_NATIVE_REVIEW") != "1" {
		t.Skip("requires explicit hardware Vulkan configuration")
	}

	driver := openCompressionReviewDevice(t)
	if !driver.compression.bc || driver.CompressedAlbedoFormat() != texturedata.BC1 {
		t.Fatal("the workspace AMD adapter must enable BC for this hardware review")
	}

	for _, dimensions := range [][2]int{{12, 12}, {12, 4}} {
		for _, space := range []texturedata.ColorSpace{texturedata.Linear, texturedata.SRGB} {
			image := reviewBC1MipTexture(t, dimensions[0], dimensions[1], space)
			values := sampleReviewTexture(t, driver, image)
			checkReviewMipColors(t, dimensions, space, values)
		}
	}

	// Reuse the maintained offline block fixtures. Decoding here admits KTX2
	// metadata and retains its prepared blocks; it does not decode texture pixels.
	for _, fixture := range []struct {
		name    string
		enabled bool
	}{
		{"bc1-srgb.ktx2", driver.compression.bc},
		{"bc6h.ktx2", driver.compression.bc},
		{"bc7.ktx2", driver.compression.bc},
		{"etc2rgb-linear.ktx2", driver.compression.etc2},
		{"etc2rgb-srgb.ktx2", driver.compression.etc2},
		{"etc2rgba-linear.ktx2", driver.compression.etc2},
		{"astc4x4-linear.ktx2", driver.compression.astc},
	} {
		if !fixture.enabled {
			t.Log("adapter does not enable", fixture.name)

			continue
		}

		payload, err := os.ReadFile("../../ktx2data/testdata/" + fixture.name)
		if err != nil {
			t.Fatal(err)
		}

		data, err := ktx2data.Decode(payload)
		if err != nil {
			t.Fatal(err)
		}

		image, err := texturedata.FromOwned(data)
		if err != nil {
			t.Fatal(err)
		}

		for _, value := range sampleReviewTexture(t, driver, image) {
			if math.IsNaN(float64(value)) || math.IsInf(float64(value), 0) {
				t.Fatal("nonfinite decoded hardware sample", fixture.name, value)
			}
		}

		texturedata.Release(image)
		t.Log("uploaded and sampled prepared fixture", fixture.name)
	}
}

func checkReviewMipColors(t *testing.T, dimensions [2]int, space texturedata.ColorSpace, values []float32) {
	t.Helper()

	for level, rgb := range [][3]float32{{1, 0, 0}, {0, 1, 0}, {0, 0, 1}, {132.0 / 255, 130.0 / 255, 132.0 / 255}} {
		for edge := range 2 {
			for channel, want := range rgb {
				if space == texturedata.SRGB {
					want = reviewSRGB(want)
				}

				got := values[(level*2+edge)*4+channel]
				if math.Abs(float64(got-want)) > 0.006 {
					t.Fatalf("%v space=%v mip=%d edge=%d channel=%d: got %v, want %v",
						dimensions, space, level, edge, channel, got, want)
				}
			}

			if got := values[(level*2+edge)*4+3]; got != 1 {
				t.Fatalf("mip %d alpha %v, want 1", level, got)
			}
		}
	}
}

func openCompressionReviewDevice(t *testing.T) *Driver {
	t.Helper()

	instance, err := wgpu.CreateInstance(nil)
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(instance.Release)

	adapter, err := instance.RequestAdapter(&wgpu.RequestAdapterOptions{PowerPreference: gputypes.PowerPreferenceHighPerformance})
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(adapter.Release)

	info, err := adapter.Info()
	if err != nil {
		t.Fatal(err)
	}

	identity := info.Vendor + " " + info.Device + " " + info.Description
	for _, software := range []string{"llvmpipe", "softpipe", "swiftshader", "lavapipe"} {
		if strings.Contains(strings.ToLower(identity), software) {
			t.Fatal("software adapter rejected", identity)
		}
	}

	if info.VendorID != 0x1002 || info.BackendType != wgpu.BackendTypeVulkan ||
		(info.AdapterType != wgpu.AdapterTypeIntegratedGPU && info.AdapterType != wgpu.AdapterTypeDiscreteGPU) {
		t.Fatal("this local check requires the workspace AMD Vulkan hardware", identity)
	}

	t.Log("hardware adapter:", identity, "backend", info.BackendType)

	driver := &Driver{instance: instance, adapter: adapter}
	if err := initializeGPUDevice(driver); err != nil {
		t.Fatal(err)
	}

	t.Cleanup(driver.device.Release)
	t.Cleanup(driver.queue.Release)
	t.Cleanup(driver.releaseMaterials)
	t.Logf("enabled compression: BC=%v ETC2=%v ASTC=%v", driver.compression.bc, driver.compression.etc2, driver.compression.astc)

	return driver
}

func reviewBC1MipTexture(t *testing.T, width, height int, space texturedata.ColorSpace) *texturedata.Texture {
	t.Helper()

	data := texturedata.Data{Width: width, Height: height, Format: texturedata.BC1, ColorSpace: space}
	// Authored BC1 blocks: RGB565 endpoints red, green, blue, then mid-gray.
	// Every selector is zero, so every block texel selects its first endpoint.
	for _, block := range [][8]byte{{0, 0xf8}, {0xe0, 7}, {0x1f, 0}, {0x10, 0x84}} {
		rowBytes, rows := texturedata.Layout(texturedata.BC1, width, height)
		pixels := make([]byte, rowBytes*rows)

		for offset := 0; offset < len(pixels); offset += len(block) {
			copy(pixels[offset:], block[:])
		}

		data.Levels = append(data.Levels, pixels)
		width, height = max(width>>1, 1), max(height>>1, 1)
	}

	image, err := texturedata.FromOwned(data)
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { texturedata.Release(image) })

	return image
}

func sampleReviewTexture(t *testing.T, driver *Driver, image *texturedata.Texture) []float32 {
	t.Helper()

	driver.device.PushErrorScope(wgpu.ErrorFilterValidation)
	defer func() {
		kind, message, err := driver.device.PopErrorScopeAsync(driver.instance)
		if err != nil || kind != wgpu.ErrorTypeNoError {
			t.Error("GPU validation", kind, message, err)
		}
	}()

	view, err := driver.textureView(image)
	if err != nil {
		t.Fatal(err)
	}

	if second, err := driver.textureView(image); err != nil || second != view || driver.textureBytes != texturedata.ByteSize(image) {
		t.Fatal("cached preparation changed residency", err, driver.textureBytes)
	}

	shader, err := driver.device.CreateShaderModuleWGSL(compressionReviewShader)
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

	levels := len(texturedata.Levels(image))
	size := uint64(levels * 2 * 4 * 4)

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
	//nolint:gosec // Texture admission bounds the mip count to 13.
	pass.DispatchWorkgroups(uint32(levels), 1, 1)
	pass.End()
	pass.Release()
	encoder.CopyBufferToBuffer(output, 0, readback, 0, size)

	commands, err := encoder.Finish()
	if err != nil {
		t.Fatal(err)
	}
	defer commands.Release()

	if _, err := driver.queue.Submit(commands); err != nil {
		t.Fatal(err)
	}

	values := readReviewSamples(t, driver, readback, size)
	driver.UnloadTexture(image)

	if len(driver.textures) != 0 || driver.textureBytes != 0 {
		t.Fatal("unload retained compressed texture residency")
	}

	return values
}

func readReviewSamples(t *testing.T, driver *Driver, buffer *wgpu.Buffer, size uint64) []float32 {
	t.Helper()

	pending, err := buffer.MapAsync(wgpu.MapModeRead, 0, size)
	if err != nil {
		t.Fatal(err)
	}
	defer pending.Release()

	deadline := time.Now().Add(10 * time.Second)

	for {
		driver.device.Poll(false)
		driver.instance.ProcessEvents()

		if done, err := pending.Status(); done {
			if err != nil {
				t.Fatal(err)
			}

			break
		}

		if time.Now().After(deadline) {
			_ = buffer.Unmap()

			t.Fatal("compressed texture readback timed out")
		}

		time.Sleep(time.Millisecond)
	}

	defer func() { _ = buffer.Unmap() }()

	pointer := buffer.GetMappedRange(0, size)
	if pointer == nil {
		t.Fatal("compressed texture readback returned no mapped range")
	}

	// MappedRange.Bytes calls wgpuBufferGetMapState, which is unimplemented in
	// the pinned native library. The completed map owns this bounded range.
	//nolint:gosec // The successful GPU map guarantees size readable bytes until the deferred Unmap.
	pixels := unsafe.Slice((*byte)(pointer), size)
	values := make([]float32, len(pixels)/4)

	for index := range values {
		values[index] = math.Float32frombits(binary.LittleEndian.Uint32(pixels[index*4:]))
	}

	return values
}

func reviewSRGB(value float32) float32 {
	if value <= 0.04045 {
		return value / 12.92
	}

	return float32(math.Pow(float64((value+0.055)/1.055), 2.4))
}
