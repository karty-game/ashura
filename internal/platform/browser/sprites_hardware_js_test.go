//go:build js && wasm

package browser

import (
	"math"
	"strings"
	"syscall/js"
	"testing"

	"github.com/karty-game/karty-webgpu/internal/diagnostics"
	"github.com/karty-game/karty-webgpu/pkg/draw2d"
)

//nolint:paralleltest // Each case isolates GPU pixel and residency evidence.
func TestSpriteHardwareExactPixelsAndAtlasClipping(t *testing.T) {
	driver := spriteHardwareDriver(t, "rgba8unorm")
	screen := draw2d.NewScreen()

	defer screen.Close()

	white := spriteHardwareImage(t, 32, 32, [4]byte{255, 255, 255, 255})
	screen.NewSprite(white).SetPosition(10, 10)

	atlas, err := draw2d.NewImage(2, 1, []byte{255, 0, 0, 255, 0, 255, 0, 255})
	if err != nil {
		t.Fatal(err)
	}

	red, err := atlas.Region(0, 0, 1, 1)
	if err != nil {
		t.Fatal(err)
	}

	screen.NewSprite(red).SetPosition(50, 10).SetScale(32, 32).SetClip(55, 15, 10, 10)
	pixels := spriteHardwarePixels(t, driver, screen, 128, 64)

	for row := range 64 {
		for column := range 128 {
			want := [4]byte{0, 0, 0, 255}
			if column >= 10 && column < 42 && row >= 10 && row < 42 {
				want = [4]byte{255, 255, 255, 255}
			}

			if column >= 55 && column < 65 && row >= 15 && row < 25 {
				want = [4]byte{255, 0, 0, 255}
			}

			assertSpritePixel(t, pixels, 128, column, row, want)
		}
	}
}

//nolint:paralleltest // Target formats deliberately run sequentially on hardware.
func TestSpriteHardwareTintOpacityAndTransforms(t *testing.T) {
	for _, format := range []string{"rgba8unorm", "rgba8unorm-srgb"} {
		t.Run(format, func(t *testing.T) {
			driver := spriteHardwareDriver(t, format)
			screen := draw2d.NewScreen()

			defer screen.Close()

			white := spriteHardwareImage(t, 1, 1, [4]byte{255, 255, 255, 255})
			screen.NewSprite(white).SetPosition(10, 10).SetScale(32, 32).SetTint(1, 0, 0).SetOrder(10)
			screen.NewSprite(white).SetPosition(20, 20).SetScale(32, 32).SetTint(0, 0, 1).SetOpacity(.5).SetOrder(20)

			gray := spriteHardwareImage(t, 1, 1, [4]byte{128, 128, 128, 128})
			screen.NewSprite(gray).SetPosition(60, 10).SetScale(16, 16).SetTint(.5, .25, 1).SetOpacity(.5)

			pair, err := draw2d.NewImage(2, 1, []byte{255, 0, 0, 255, 0, 255, 0, 255})
			if err != nil {
				t.Fatal(err)
			}

			screen.NewSprite(pair).SetPosition(20, 60).SetScale(-8, 8)
			screen.NewSprite(pair).SetPosition(88, 40).SetScale(8, 8).SetRotation(math.Pi / 2)
			pixels := spriteHardwarePixels(t, driver, screen, 96, 80)

			mixed := byte(128)
			if format == "rgba8unorm-srgb" {
				mixed = byte(math.Round(spriteEncode(.5) * 255))
			}

			assertSpritePixel(t, pixels, 96, 12, 12, [4]byte{255, 0, 0, 255})
			assertSpritePixel(t, pixels, 96, 25, 25, [4]byte{mixed, 0, mixed, 255})
			assertSpritePixel(t, pixels, 96, 5, 61, [4]byte{0, 255, 0, 255})
			assertSpritePixel(t, pixels, 96, 18, 61, [4]byte{255, 0, 0, 255})
			assertSpritePixel(t, pixels, 96, 83, 42, [4]byte{255, 0, 0, 255})
			assertSpritePixel(t, pixels, 96, 83, 52, [4]byte{0, 255, 0, 255})

			var tinted [4]byte

			for channel, tint := range [3]float64{.5, .25, 1} {
				linear := spriteDecode(128.0/255) * tint
				alpha := 128.0 / 255 * .5

				value := spriteEncode(linear) * alpha
				if format == "rgba8unorm-srgb" {
					value = spriteEncode(linear * alpha)
				}

				tinted[channel] = byte(math.Round(value * 255))
			}

			tinted[3] = 255
			assertSpritePixel(t, pixels, 96, 64, 14, tinted)
		})
	}
}

//nolint:paralleltest // Mutations exercise one backend's retained revision history.
func TestSpriteHardwareRetainsPagesAndRecoversSkippedFrames(t *testing.T) {
	driver := spriteHardwareDriver(t, "rgba8unorm")

	feature, ok := driver.spriteBridge.(*spriteFeature)
	if !ok {
		t.Fatal("sprite feature was not registered")
	}

	screen := draw2d.NewScreen()

	defer screen.Close()

	image := spriteHardwareImage(t, 1, 1, [4]byte{255, 255, 255, 255})
	sprite := screen.NewSprite(image).SetPosition(10, 10).SetScale(8, 8)
	spriteHardwarePixels(t, driver, screen, 96, 64)
	frame := screen.Frame(96, 64)
	page := feature.sprites.pages[frame.Textures[0]].texture
	buffer := feature.sprites.buffer
	before := screen.Stats()
	spriteHardwarePixels(t, driver, screen, 96, 64)

	if screen.Stats() != before || !buffer.Equal(feature.sprites.buffer) {
		t.Fatal("idle frame rebuilt retained sprite data")
	}

	sprite.SetVisible(false)
	spriteHardwarePixels(t, driver, screen, 96, 64)

	if len(feature.sprites.pages) != 1 || !feature.sprites.pages[frame.Textures[0]].texture.Equal(page) {
		t.Fatal("hiding a live sprite evicted its prepared page")
	}

	sprite.SetVisible(true).SetPosition(20, 20)
	screen.Frame(96, 64) // Deliberately skip this source revision in the backend.
	sprite.SetPosition(30, 30)

	pixels := spriteHardwarePixels(t, driver, screen, 96, 64)
	assertSpritePixel(t, pixels, 96, 31, 31, [4]byte{255, 255, 255, 255})
	assertSpritePixel(t, pixels, 96, 11, 11, [4]byte{0, 0, 0, 255})

	if !feature.sprites.pages[frame.Textures[0]].texture.Equal(page) {
		t.Fatal("showing/moving a sprite uploaded its page again")
	}

	sprite.Destroy()
	spriteHardwarePixels(t, driver, screen, 96, 64)

	if len(feature.sprites.pages) != 0 || len(feature.sprites.batches) != 0 {
		t.Fatal("destroyed last sprite retained backend pages or draws")
	}
}

func spriteHardwareDriver(t *testing.T, format string) *Driver {
	t.Helper()

	navigator := js.Global().Get("navigator")
	if navigator.IsUndefined() || navigator.Get("gpu").IsUndefined() {
		t.Skip("requires browser WebGPU hardware")
	}

	adapter, err := awaitPromise(navigator.Get("gpu").Call("requestAdapter"))
	if err != nil || adapter.IsNull() || adapter.IsUndefined() {
		t.Fatal("hardware adapter unavailable", err)
	}

	info := adapter.Get("info")
	identity := info.Get("vendor").String() + " " + info.Get("architecture").String() + " " + info.Get("description").String()

	for _, software := range []string{"swiftshader", "llvmpipe", "softpipe", "lavapipe"} {
		if strings.Contains(strings.ToLower(identity), software) {
			t.Fatal("software adapter rejected", identity)
		}
	}

	if fallback := info.Get("isFallbackAdapter"); fallback.Type() == js.TypeBoolean && fallback.Bool() {
		t.Fatal("fallback adapter rejected", identity)
	}

	t.Log("GPU adapter:", identity, "target:", format)

	device, err := awaitPromise(adapter.Call("requestDevice"))
	if err != nil {
		t.Fatal(err)
	}

	driver := &Driver{device: device, queue: device.Get("queue"), colorFormat: format, metrics: &diagnostics.Monitor{}}

	installTestFrameHost(t)

	host, err := newFrameHost()
	if err != nil {
		t.Fatal(err)
	}

	driver.frameHost, driver.frameEncoder = host, host.Get("encoder")
	driver.renderPass, driver.surfaceView = host.Get("renderPass"), host.Get("surface")

	t.Cleanup(func() {
		driver.closeSprites()
		host.Call("close")
		device.Call("destroy")
	})

	if err := RegisterSprites(driver); err != nil {
		t.Fatal(err)
	}

	return driver
}

func spriteHardwareImage(t *testing.T, width, height int, color [4]byte) *draw2d.Image {
	t.Helper()

	pixels := make([]byte, width*height*4)
	for offset := 0; offset < len(pixels); offset += 4 {
		copy(pixels[offset:], color[:])
	}

	image, err := draw2d.NewImage(width, height, pixels)
	if err != nil {
		t.Fatal(err)
	}

	return image
}

func spriteHardwarePixels(t *testing.T, driver *Driver, screen *draw2d.Screen, width, height int) []byte {
	t.Helper()

	if err := driver.updateSprites(screen.Frame(width, height)); err != nil {
		t.Fatal(err)
	}

	texture := driver.device.Call("createTexture", jsObject(map[string]any{
		"size": []any{width, height}, "format": driver.colorFormat, "usage": 0x11, // RENDER_ATTACHMENT | COPY_SRC.
	}))
	defer texture.Call("destroy")

	context := js.Global().Get("Function").New("texture", "return {getCurrentTexture() {return texture; }}").Invoke(texture)
	if !driver.frameHost.Call("begin", context, driver.device).Bool() {
		t.Fatal("failed to begin sprite readback frame")
	}

	view, encoder := driver.surfaceView, driver.frameEncoder
	clearPass := driver.startRenderPass(encoder, jsObject(map[string]any{"colorAttachments": []any{
		jsObject(map[string]any{"view": view, "loadOp": "clear", "storeOp": "store", "clearValue": []any{0, 0, 0, 1}}),
	}}))
	clearPass.Call("end")
	driver.drawSprites(encoder, view)

	if err := driver.submit(); err != nil {
		t.Fatal(err)
	}

	row := (width*4 + 255) / 256 * 256
	buffer := driver.device.Call("createBuffer", jsObject(map[string]any{"size": row * height, "usage": 9})) // MAP_READ | COPY_DST.

	defer buffer.Call("destroy")

	readbackEncoder := driver.device.Call("createCommandEncoder")
	readbackEncoder.Call("copyTextureToBuffer", jsObject(map[string]any{"texture": texture}),
		jsObject(map[string]any{"buffer": buffer, "bytesPerRow": row, "rowsPerImage": height}), []any{width, height, 1})
	driver.queue.Call("submit", []any{readbackEncoder.Call("finish")})

	if _, err := awaitPromise(buffer.Call("mapAsync", 1)); err != nil {
		t.Fatal(err)
	}

	bytes := make([]byte, row*height)
	js.CopyBytesToGo(bytes, js.Global().Get("Uint8Array").New(buffer.Call("getMappedRange")))
	buffer.Call("unmap")

	if err := driver.deviceFailure(); err != nil {
		t.Fatal(err)
	}

	pixels := make([]byte, width*height*4)
	for y := range height {
		copy(pixels[y*width*4:(y+1)*width*4], bytes[y*row:y*row+width*4])
	}

	return pixels
}

func assertSpritePixel(t *testing.T, pixels []byte, width, column, row int, want [4]byte) {
	t.Helper()

	for channel, value := range want {
		got := pixels[(row*width+column)*4+channel]
		if math.Abs(float64(got)-float64(value)) > 2 {
			t.Fatalf("pixel (%d,%d) channel %d = %d, want %d", column, row, channel, got, value)
		}
	}
}

func spriteDecode(value float64) float64 {
	if value <= .04045 {
		return value / 12.92
	}

	return math.Pow((value+.055)/1.055, 2.4)
}

func spriteEncode(value float64) float64 {
	if value <= .0031308 {
		return value * 12.92
	}

	return 1.055*math.Pow(value, 1/2.4) - .055
}
