//go:build linux && ashura_ui_native_review

package native

import (
	"context"
	"errors"
	"os"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/karty-game/ashura/internal/diagnostics"
	"github.com/karty-game/ashura/internal/platform"
	"github.com/karty-game/ashura/internal/render"
	"github.com/karty-game/ashura/pkg/draw2d"
)

//nolint:paralleltest // Owns SDL and a native GPU surface on the main thread.
func TestScreen2DHardwarePresentation(t *testing.T) {
	if os.Getenv("ASHURA_SCREEN2D_NATIVE_REVIEW") != "1" {
		t.Skip("requires explicit hardware Vulkan and display configuration")
	}

	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	image, err := draw2d.NewImage(1, 1, []byte{255, 255, 255, 255})
	if err != nil {
		t.Fatal(err)
	}

	screen := draw2d.NewScreen()
	defer screen.Close()

	screen.NewSprite(image).SetPosition(10, 10).SetScale(32, 32)
	backend, err := New(platform.Config{
		Title: "Ashura Screen2D native review", Width: 128, Height: 96,
		SampleCount: 4, Metrics: &diagnostics.Monitor{},
		Features: []platform.Feature{{Kind: platform.SpriteFeature, Install: func(host any, _ platform.FeatureConfig) error {
			return RegisterSprites(host)
		}}},
	})
	if err != nil {
		t.Fatal(err)
	}

	driver, ok := backend.(*Driver)
	if !ok {
		t.Fatal("unexpected native driver")
	}

	defer func() {
		if err := driver.Close(); err != nil {
			t.Error(err)
		}
	}()

	info, err := driver.adapter.Info()
	if err != nil {
		t.Fatal(err)
	}

	identity := info.Vendor + " " + info.Device + " " + info.Description
	for _, software := range []string{"llvmpipe", "softpipe", "swiftshader", "lavapipe"} {
		if strings.Contains(strings.ToLower(identity), software) {
			t.Fatal("software adapter rejected", identity)
		}
	}

	if info.VendorID != 0x1002 {
		t.Fatal("this local check requires the workspace AMD adapter", identity)
	}

	t.Log("hardware adapter:", identity, info.BackendType)
	frames := 0
	err = driver.Run(context.Background(), func(_ time.Duration, input platform.InputState) (render.Frame, error) {
		frames++
		if frames > 5 {
			return render.Frame{}, context.Canceled
		}

		return render.Frame{Screen2D: screen.Frame(input.Width, input.Height), ClearColor: [4]float32{0, 0, 0, 1}}, nil
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}

	feature, ok := driver.spriteBridge.(*spriteFeature)
	if !ok || feature.sprites == nil || feature.sprites.revision == 0 || len(feature.sprites.pages) != 1 ||
		len(feature.sprites.batches) != 1 {
		t.Fatal("native Screen2D commands were not presented")
	}

	if driver.depthTexture != nil || driver.depthView != nil || driver.colorTexture != nil || driver.colorView != nil {
		t.Fatal("2D-only presentation allocated scene depth/MSAA targets")
	}

	t.Log("presented five native sprite frames; command-path evidence only, no pixel readback")
}
