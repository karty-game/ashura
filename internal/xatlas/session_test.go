//go:build !js

package xatlas

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/karty-game/karty-webgpu/internal/modeldata"
)

func TestPortableSessionRejectsInvalidContext(t *testing.T) {
	t.Parallel()

	//nolint:staticcheck // Intentionally exercise invalid nil context admission.
	if _, err := OpenSession(nil); !errors.Is(err, errFormat) {
		t.Fatal("accepted nil context", err)
	}

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	if _, err := OpenSession(ctx); !errors.Is(err, context.Canceled) {
		t.Fatal("accepted canceled context", err)
	}
}

func TestPortableSessionConcurrentInitializationAndRequests(t *testing.T) {
	t.Parallel()

	session, err := OpenSession(t.Context())
	if err != nil {
		t.Fatal(err)
	}

	defer func() { _ = session.Close() }()

	parts := []modeldata.Part{pageTestQuad("shared", 0, 1)}

	type completed struct {
		value Atlas
		err   error
	}

	const requests = 8

	results := make(chan completed, requests)
	start := make(chan struct{})

	for range requests {
		go func() {
			<-start

			value, err := session.Generate(t.Context(), parts, 64, 4)
			results <- completed{value, err}
		}()
	}

	close(start)

	var first Atlas

	for index := range requests {
		result := <-results
		if result.err != nil {
			t.Error(result.err)

			continue
		}

		if index == 0 {
			first = result.value
		} else if !reflect.DeepEqual(first, result.value) {
			t.Error("concurrent portable requests changed deterministic atlas")
		}
	}
}
