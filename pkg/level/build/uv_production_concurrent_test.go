//go:build !js

package levelbuild

import (
	"context"
	"errors"
	"testing"

	preparation "github.com/karty-game/karty-webgpu/internal/lightmapbuild"
)

func TestProductionUVSharedPreparerConcurrentLifetime(t *testing.T) {
	t.Parallel()

	plugin := &uvPlugin{}

	t.Cleanup(func() {
		if err := plugin.closeProductionPreparer(); err != nil {
			t.Error(err)
		}
	})

	canceled, cancel := context.WithCancel(t.Context())
	cancel()

	if _, err := plugin.productionPreparer(canceled); !errors.Is(err, context.Canceled) {
		t.Fatal("canceled setup unexpectedly succeeded", err)
	}

	type prepared struct {
		value *preparation.UVPreparer
		err   error
	}

	const requests = 16

	results := make(chan prepared, requests)
	start := make(chan struct{})

	for range requests {
		go func() {
			<-start

			value, err := plugin.productionPreparer(t.Context())
			results <- prepared{value: value, err: err}
		}()
	}

	close(start)

	var shared *preparation.UVPreparer

	for range requests {
		result := <-results
		if result.err != nil {
			t.Error(result.err)

			continue
		}

		if shared == nil {
			shared = result.value
		} else if result.value != shared {
			t.Error("parallel UV actions acquired different build helpers")
		}
	}

	if t.Failed() {
		return
	}

	// Join all users before cleanup, matching the asset coordinator's lifecycle.
	if err := plugin.closeProductionPreparer(); err != nil {
		t.Fatal(err)
	}

	if err := plugin.closeProductionPreparer(); err != nil {
		t.Fatal("repeated cleanup failed", err)
	}

	next, err := plugin.productionPreparer(t.Context())
	if err != nil || next == shared {
		t.Fatal("a later build retained the closed helper", err)
	}
}
