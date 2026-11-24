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
	"github.com/karty-game/ashura/internal/uihost"
	"github.com/karty-game/ashura/pkg/ui"
	"github.com/karty-game/ashura/pkg/ui/fonts/defaultfont"
)

//nolint:paralleltest // Owns SDL and a native GPU surface on the main thread.
func TestUIHardwarePresentation(t *testing.T) {
	if os.Getenv("ASHURA_UI_NATIVE_REVIEW") != "1" {
		t.Skip("requires explicit hardware Vulkan and display configuration")
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	face, err := defaultfont.Load()
	if err != nil {
		t.Fatal(err)
	}
	button := ui.NewButton("Native WebGPU", nil)
	view, err := ui.New(ui.Column(ui.NewText("MSDF text"), button), face)
	if err != nil {
		t.Fatal(err)
	}
	backend, err := New(
		platform.Config{
			Title:       "Ashura UI hardware review",
			Width:       500,
			Height:      400,
			SampleCount: 4,
			Metrics:     &diagnostics.Monitor{},
			UI:          nativeReviewAttachment{view},
		},
	)
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
	err = driver.Run(context.Background(), func(_ time.Duration, _ platform.InputState) (render.Frame, error) {
		frames++
		if frames > 5 {
			return render.Frame{}, context.Canceled
		}
		if frames == 2 {
			view.SetFocus(button)
			view.Dispatch(ui.Event{Kind: ui.KeyDown, Key: " "})
			view.Dispatch(ui.Event{Kind: ui.KeyUp, Key: " "})
		}
		frame, err := view.Frame(time.Duration(frames) * time.Second / 60)
		return render.Frame{UI: frame, ClearColor: [4]float32{0.02, 0.03, 0.05, 1}}, err
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	bridge, ok := driver.uiBridge.(*nativeUIHost)
	if !ok || bridge.uiGPU == nil || bridge.uiGPU.revision == 0 || len(bridge.uiGPU.pages) != 1 {
		t.Fatal("UI was not presented")
	}
}

// Keep this backend fixture independent of the frontend package importing it.
type nativeReviewAttachment struct{ *ui.View }

func (attachment nativeReviewAttachment) AttachUI(host any) (uihost.Bridge, error) {
	return AttachUI(host, attachment.View)
}
