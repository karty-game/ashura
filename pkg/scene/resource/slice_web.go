//go:build js && wasm

package resource

import (
	"context"
	"syscall/js"
)

func verifySliceChecksum(ctx context.Context, data []byte, expected string) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	input := js.Global().Get("Uint8Array").New(len(data))
	if js.CopyBytesToJS(input, data) != len(data) {
		return ErrInvalid
	}

	return verifyBrowserChecksum(ctx, input, expected)
}
