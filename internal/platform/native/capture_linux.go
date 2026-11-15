//go:build linux

package native

import (
	"context"
	"image"
)

// Capture is not available until the asynchronous readback path is implemented.
func (driver *Driver) Capture(ctx context.Context) (*image.RGBA, error) {
	_ = ctx

	return nil, errCaptureUnsupported
}
