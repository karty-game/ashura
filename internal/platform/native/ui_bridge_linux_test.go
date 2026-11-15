//go:build linux

package native

import (
	"errors"
	"testing"

	"github.com/karty-game/karty-webgpu/internal/uipaint"
)

func TestUIFrameRequiresNativeAttachment(t *testing.T) {
	t.Parallel()

	driver := &Driver{}
	if err := driver.updateAttachedUI(&uipaint.Frame{}); !errors.Is(err, errEmptySurfaceFrame) {
		t.Fatalf("unattached UI frame was not rejected: %v", err)
	}

	if err := driver.updateAttachedUI(nil); err != nil {
		t.Fatalf("plain scene required a UI attachment: %v", err)
	}
}

func TestNativeUIBridgeClosesIndependently(t *testing.T) {
	t.Parallel()

	driver := &Driver{}
	bridge := &nativeUIHost{Driver: driver}
	bridge.Close()
	bridge.Close()

	if driver.closed {
		t.Fatal("UI bridge closed the owning native driver")
	}
}
