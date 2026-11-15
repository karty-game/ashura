//go:build !js

package xatlas

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sync"
	"testing"
	"time"

	"github.com/tetratelabs/wazero"
)

// repairStartedContext observes the first successful poll without changing its
// cancellation semantics. It lets the test cancel after host repair starts.
type repairStartedContext struct {
	//nolint:containedctx // Test-only request context observer, never retained by a build.
	context.Context

	started chan struct{}
	once    sync.Once
}

func (ctx *repairStartedContext) Err() error {
	err := ctx.Context.Err()
	if err == nil {
		ctx.once.Do(func() { close(ctx.started) })
	}

	return err
}

func TestWASIHostRepairStopsAfterActiveCancellation(t *testing.T) {
	t.Parallel()

	const faceCount = uint32(1 << 16)

	mesh := rawMesh{vertices: make([]rawVertex, faceCount*3), indices: make([]uint32, faceCount*3)}
	faces := make([]uint32, faceCount)

	for face := range faceCount {
		left := float32(face * 2)
		mesh.vertices[face*3].uv = [2]float32{left, 0}
		mesh.vertices[face*3+1].uv = [2]float32{left + 1, 0}
		mesh.vertices[face*3+2].uv = [2]float32{left, 1}
		faces[face] = face

		for corner := range uint32(3) {
			mesh.indices[face*3+corner] = face*3 + corner
		}
	}

	bounded, cancel := context.WithCancel(t.Context())
	defer cancel()

	ctx := &repairStartedContext{Context: bounded, started: make(chan struct{})}
	result := make(chan error, 1)

	go func() {
		defer func() {
			failure := recover()
			if cause, ok := failure.(error); ok {
				result <- cause
			} else {
				result <- fmt.Errorf("host repair did not abort with a context error: %v: %w", failure, errFormat)
			}
		}()

		_ = chartOverlaps(ctx, mesh, faces)
	}()

	select {
	case <-ctx.started:
		cancel()
	case <-time.After(time.Second):
		t.Fatal("host repair never polled the request")
	}

	select {
	case err := <-result:
		if !errors.Is(err, context.Canceled) {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("host repair ignored active cancellation")
	}
}

func TestWASIGuestChartMetadataCannotMultiplyFaceAllocations(t *testing.T) {
	t.Parallel()

	session := wasiSession{}

	t.Cleanup(func() { _ = session.close() })

	if err := session.prepare(t.Context()); err != nil {
		t.Fatal(err)
	}

	module, err := session.runtime.InstantiateModule(
		t.Context(), session.compiled, wazero.NewModuleConfig().WithName("").WithStartFunctions(),
	)
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { _ = module.Close(context.Background()) })

	guest := atlasGuest{t.Context(), module}
	charts, faces := guest.alloc(40), guest.alloc(8)
	guest.put(charts, faces)
	guest.put(charts+20, faces)
	guest.put(faces, 0)
	guest.put(faces+4, 0)

	// Two source triangles permit at most two chart face entries. Reject huge
	// counts before allocation, cumulative overflow and duplicate membership.
	for _, counts := range [][2]uint32{{0, math.MaxUint32}, {1, 2}, {1, 1}, {2, 0}} {
		guest.put(charts+8, counts[0])
		guest.put(charts+28, counts[1])

		mesh := rawMesh{charts: make([][]uint32, 2)}

		expectGuestFormatPanic(t, func() { guest.readCharts(&mesh, charts, 6) })
	}
}

func expectGuestFormatPanic(t *testing.T, operation func()) {
	t.Helper()

	defer func() {
		failure := recover()

		cause, ok := failure.(error)
		if !ok || !errors.Is(cause, errFormat) {
			t.Fatalf("malformed guest metadata did not fail admission: %v", failure)
		}
	}()

	operation()
}
