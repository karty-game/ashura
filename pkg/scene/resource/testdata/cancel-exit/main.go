//go:build js && wasm

package main

import (
	"context"
	"errors"
	"strings"
	"syscall/js"

	"github.com/karty-game/ashura/pkg/scene/resource"
)

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	started := make(chan struct{})
	notify := js.FuncOf(func(js.Value, []js.Value) any {
		close(started)

		return nil
	})
	defer notify.Release()
	js.Global().Set("onReadStarted", notify)
	defer js.Global().Set("onReadStarted", js.Undefined())

	completed := make(chan error, 1)
	go func() {
		_, err := resource.Read(ctx, "/fixture", resource.Asset{
			ID: "pending", Path: "prepared.bin", Size: 8, SHA256: strings.Repeat("0", 64),
		})
		completed <- err
	}()
	<-started
	cancel()
	if err := <-completed; !errors.Is(err, context.Canceled) {
		panic("resource cancellation did not finish before Promise settlement")
	}
	js.Global().Set("readFinished", true)
}
