package render_test

import (
	"encoding/binary"
	"errors"
	"math"
	"testing"

	"github.com/karty-game/karty-webgpu/internal/render"
)

var errGPUAllocation = errors.New("GPU allocation failed")

func triangle() render.DrawCall {
	return render.DrawCall{
		Vertices: []render.Vertex{{Position: [3]float32{1, 0, 0}}, {Position: [3]float32{0, 1, 0}}, {}},
		Indices:  []uint32{0, 1, 2},
	}
}

func TestGeometryUploadsAndRetirement(t *testing.T) {
	t.Parallel()

	var cache render.GeometryCache[int]

	created, released := 0, 0
	create := func(vertices, indices, _ []byte) (int, error) {
		created++

		if len(vertices) != 3*render.VertexStride || len(indices) != 3*render.IndexSize ||
			binary.LittleEndian.Uint32(vertices) != math.Float32bits(1) || binary.LittleEndian.Uint32(indices[8:]) != 2 {
			t.Fatal("upload did not contain the expected triangle")
		}

		return created, nil
	}
	release := func(int) { released++ }
	draw := triangle()

	first, err := cache.Resolve(draw, create)
	if err != nil {
		t.Fatal(err)
	}

	cache.Prune(release)

	for frame := range 100 {
		draw.ID = uint32(frame)
		draw.Transform[12] = float32(frame)

		draw.Source = frame%2 == 0
		if resource, resolveErr := cache.Resolve(draw, create); resolveErr != nil || resource != first {
			t.Fatalf("camera/material/instance update replaced geometry: %d, %v", resource, resolveErr)
		}

		cache.Prune(release)
	}

	if created != 1 || released != 0 {
		t.Fatalf("static geometry: uploads=%d, releases=%d", created, released)
	}

	// One remaining instance keeps the shared upload alive. Replacing slices
	// selects a new upload while retiring the old one after this submission.
	draw.Vertices = append([]render.Vertex(nil), draw.Vertices...)
	if _, err = cache.Resolve(draw, create); err != nil {
		t.Fatal(err)
	}

	cache.Prune(release)

	if created != 2 || released != 1 {
		t.Fatalf("replacement: uploads=%d, releases=%d", created, released)
	}

	cache.Prune(release)
	cache.Clear(release)
	cache.Clear(release)

	if released != created {
		t.Fatalf("removal/close leaked or double-released resources: uploads=%d, releases=%d", created, released)
	}
}

func TestGeometryAdmissionBeforeGPUAllocation(t *testing.T) {
	t.Parallel()

	for _, indices := range [][]uint32{nil, {0, 1}, {0, 1, 3}} {
		var cache render.GeometryCache[int]

		draw := triangle()
		draw.Indices = indices

		_, err := cache.Resolve(draw, func(_, _, _ []byte) (int, error) {
			t.Fatal("GPU allocation attempted before complete index validation")

			return 0, nil
		})
		if err == nil {
			t.Fatalf("accepted invalid indices %v", indices)
		}
	}
}

func TestGeometryCreationFailureCanRetry(t *testing.T) {
	t.Parallel()

	var cache render.GeometryCache[int]

	draw := triangle()
	failure := errGPUAllocation

	_, err := cache.Resolve(draw, func(_, _, _ []byte) (int, error) { return 0, failure })
	if !errors.Is(err, failure) {
		t.Fatalf("creation error = %v", err)
	}

	resource, err := cache.Resolve(draw, func(_, _, _ []byte) (int, error) { return 1, nil })
	if err != nil || resource != 1 {
		t.Fatalf("failed upload poisoned cache: resource=%d, err=%v", resource, err)
	}

	released := 0

	cache.Clear(func(int) { released++ })

	if released != 1 {
		t.Fatalf("Close released %d resources, want 1", released)
	}
}

func TestGeometryBatchAdmissionAndSharedUpload(t *testing.T) {
	t.Parallel()

	var cache render.GeometryCache[int]

	good, bad := triangle(), triangle()

	bad.Indices = []uint32{0, 1, 3}
	if err := cache.Prepare([]render.DrawCall{good, bad}); err == nil {
		t.Fatal("accepted invalid late draw")
	}

	cache.Clear(func(int) { t.Fatal("failed batch owned GPU resources") })

	if err := cache.Prepare([]render.DrawCall{good, good}); err != nil {
		t.Fatal(err)
	}

	uploads := 0

	for range 2 {
		_, err := cache.Resolve(good, func(_, _, _ []byte) (int, error) {
			uploads++

			return uploads, nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}

	if uploads != 1 {
		t.Fatalf("shared geometry uploaded %d times", uploads)
	}

	cache.Clear(func(int) {})
}

//nolint:paralleltest // AllocsPerRun must be isolated on Go 1.27.
func TestGeometryCacheHitDoesNotAllocate(t *testing.T) {
	var cache render.GeometryCache[int]

	draw := triangle()
	created := 0

	//nolint:unparam // Backend creation callbacks must include an error result.
	create := func(_, _, _ []byte) (int, error) {
		created++

		return 1, nil
	}
	if _, err := cache.Resolve(draw, create); err != nil {
		t.Fatal(err)
	}

	allocations := testing.AllocsPerRun(100, func() {
		if _, err := cache.Resolve(draw, create); err != nil {
			t.Fatal(err)
		}

		cache.Prune(func(int) { t.Fatal("live geometry retired") })
	})
	if allocations != 0 || created != 1 {
		t.Fatalf("cache hit: allocations=%v, uploads=%d", allocations, created)
	}
}
