//go:build js && wasm

package browser

import (
	"syscall/js"
	"testing"

	"github.com/karty-game/karty-webgpu/internal/texturedata"
)

func TestCompressedMipUploadsUsePhysicalBlockExtents(t *testing.T) {
	t.Parallel()

	navigator := js.Global().Get("navigator")
	if navigator.IsUndefined() || navigator.IsNull() {
		t.Skip("browser navigator is unavailable")
	}

	gpu := navigator.Get("gpu")
	if gpu.IsUndefined() || gpu.IsNull() {
		t.Skip("browser WebGPU is unavailable")
	}

	for _, test := range []struct {
		name    string
		feature string
		format  texturedata.Format
	}{
		{"BC1", "texture-compression-bc", texturedata.BC1},
		{"ETC2", "texture-compression-etc2", texturedata.ETC2RGB},
		{"ETC2RGBA", "texture-compression-etc2", texturedata.ETC2RGBA},
		{"ASTC4x4", "texture-compression-astc", texturedata.ASTC4x4},
		{"BC6H", "texture-compression-bc", texturedata.BC6H},
		{"BC7", "texture-compression-bc", texturedata.BC7},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			device := compressedDeviceForTest(t, gpu, test.feature)
			defer device.Call("destroy")

			assertCompressedPhysicalMipUpload(t, device, test.format)
		})
	}
}

func compressedDeviceForTest(t *testing.T, gpu js.Value, feature string) js.Value {
	t.Helper()

	adapter, err := awaitPromise(gpu.Call("requestAdapter"))
	if err != nil {
		t.Fatal(err)
	}

	if adapter.IsUndefined() || adapter.IsNull() {
		t.Skip("browser WebGPU adapter is unavailable")
	}

	if !adapter.Get("features").Call("has", feature).Bool() {
		t.Skip("adapter does not support " + feature)
	}

	device, err := awaitPromise(adapter.Call("requestDevice", jsObject(map[string]any{
		"requiredFeatures": []any{feature},
	})))
	if err != nil {
		t.Fatal(err)
	}

	return device
}

func assertCompressedPhysicalMipUpload(t *testing.T, device js.Value, format texturedata.Format) {
	t.Helper()

	space := texturedata.SRGB
	if format == texturedata.BC6H || format == texturedata.BC7 || format == texturedata.ETC2RGBA || format == texturedata.ASTC4x4 {
		space = texturedata.Linear
	}

	levels := make([][]byte, 0, 4)

	var total uint64

	for _, size := range []int{12, 6, 3, 1} {
		row, rows := texturedata.Layout(format, size, size)
		levels = append(levels, make([]byte, row*rows))
		total += uint64(row * rows) //nolint:gosec // Fixed 12px fixture dimensions bound the product to 144 bytes.
	}

	image, err := texturedata.FromOwned(texturedata.Data{
		Width:      12,
		Height:     12,
		ColorSpace: space,
		Format:     format,
		Levels:     levels,
	})
	if err != nil {
		t.Fatal(err)
	}

	defer texturedata.Release(image)

	driver := &Driver{device: device, queue: device.Get("queue")}
	device.Call("pushErrorScope", "validation")

	view, uploadErr := driver.textureView(image)
	validation, scopeErr := awaitPromise(device.Call("popErrorScope"))

	if uploadErr != nil {
		t.Fatal(uploadErr)
	}

	if scopeErr != nil {
		t.Fatal(scopeErr)
	}

	if !validation.IsNull() && !validation.IsUndefined() {
		t.Fatal(validation.Get("message").String())
	}

	if view.IsNull() || view.IsUndefined() {
		t.Fatal("compressed texture view is unavailable")
	}

	if _, err := awaitPromise(driver.queue.Call("onSubmittedWorkDone")); err != nil {
		t.Fatal(err)
	}

	if len(driver.textures) != 1 || driver.textureBytes != total {
		t.Fatal("compressed upload residency does not match complete physical blocks")
	}
}
