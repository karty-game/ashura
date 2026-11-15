package render

import (
	"errors"
	"fmt"
	"math"

	"github.com/karty-game/karty-webgpu/internal/meshdata"
)

var (
	errGeometryCount = errors.New("draw geometry exceeds supported vertex/index bounds")
	errGeometryIndex = errors.New("draw index references a vertex outside the position array")
)

type geometryKey struct {
	prepared    *meshdata.Geometry
	vertices    *Vertex
	indices     *uint32
	vertexCount int
	indexCount  int
}

type geometryEntry[T any] struct {
	resource T
	ready    bool
	seen     bool
}

// GeometryCache owns backend resources for immutable geometry slices. The zero
// value is ready for use. Prune must run after submission so retired resources
// are no longer referenced by an unsubmitted command buffer.
type GeometryCache[T any] struct {
	entries map[geometryKey]*geometryEntry[T]
}

// Prepare admits the entire frame before any geometry GPU allocations/uploads.
// Only new slice identities require index validation; cache hits inspect headers.
func (cache *GeometryCache[T]) Prepare(draws []DrawCall) error {
	for _, draw := range draws {
		if !HasGeometry(draw) {
			continue
		}

		if err := validateInstanceCount(draw); err != nil {
			return err
		}

		if _, err := cache.entry(draw); err != nil {
			return err
		}
	}

	return nil
}

// Resolve packs and uploads admitted geometry only once. Material, camera and
// transform changes do not affect the identity of immutable geometry slices.
func (cache *GeometryCache[T]) Resolve(draw DrawCall, create func(vertices, indices, lightmapUV []byte) (T, error)) (T, error) {
	cached, err := cache.entry(draw)
	if err != nil {
		var zero T

		return zero, err
	}

	if !cached.ready {
		var vertices, indices []byte
		if draw.Prepared != nil {
			vertices, indices = draw.Prepared.Vertices(), draw.Prepared.Indices()
		} else {
			vertices = AppendVertices(make([]byte, 0, len(draw.Vertices)*VertexStride), draw.Vertices)
			indices = AppendIndices(make([]byte, 0, len(draw.Indices)*IndexSize), draw.Indices)
		}

		var lightmapUV []byte
		if draw.Prepared != nil {
			lightmapUV = draw.Prepared.LightmapUV()
		}

		resource, createErr := create(vertices, indices, lightmapUV)
		if createErr != nil {
			var zero T

			return zero, createErr
		}

		cached.resource, cached.ready = resource, true
	}

	cached.seen = true

	return cached.resource, nil
}

func (cache *GeometryCache[T]) entry(draw DrawCall) (*geometryEntry[T], error) {
	if !HasGeometry(draw) {
		return nil, errGeometryCount
	}

	key := geometryKey{prepared: draw.Prepared}
	if draw.Prepared == nil {
		key.vertices, key.indices, key.vertexCount, key.indexCount = &draw.Vertices[0], &draw.Indices[0], len(
			draw.Vertices,
		), len(
			draw.Indices,
		)
	}

	if cached, ok := cache.entries[key]; ok {
		return cached, nil
	}

	if draw.Prepared == nil {
		if err := validateGeometry(draw); err != nil {
			return nil, err
		}
	}

	if cache.entries == nil {
		cache.entries = make(map[geometryKey]*geometryEntry[T])
	}

	cached := &geometryEntry[T]{}
	cache.entries[key] = cached

	return cached, nil
}

func validateGeometry(draw DrawCall) error {
	const triangleIndices = 3
	if uint64(len(draw.Vertices)) > math.MaxUint32 || uint64(len(draw.Indices)) > math.MaxUint32 ||
		len(draw.Vertices) > math.MaxInt/VertexStride || len(draw.Indices) > math.MaxInt/IndexSize ||
		len(draw.Indices)%triangleIndices != 0 {
		return errGeometryCount
	}

	for _, index := range draw.Indices {
		if uint64(index) >= uint64(len(draw.Vertices)) {
			return fmt.Errorf("%w: index %d, vertices %d", errGeometryIndex, index, len(draw.Vertices))
		}
	}

	return nil
}

// Prune releases resources unused by this frame and resets the live markers.
// The backend release operation retains submitted GPU work without a CPU wait.
func (cache *GeometryCache[T]) Prune(release func(T)) {
	for key, cached := range cache.entries {
		if !cached.seen {
			if cached.ready {
				release(cached.resource)
			}

			delete(cache.entries, key)
		}

		cached.seen = false
	}
}

// Clear releases all resources at the App owner boundary.
func (cache *GeometryCache[T]) Clear(release func(T)) {
	for key, cached := range cache.entries {
		if cached.ready {
			release(cached.resource)
		}

		delete(cache.entries, key)
	}
}

// HasGeometry reports whether a draw contains an admitted packed mesh or slices.
func HasGeometry(draw DrawCall) bool {
	return draw.Prepared != nil || len(draw.Vertices) != 0 && len(draw.Indices) != 0
}

// IndexCount returns the immutable geometry's index count without decoding it.
func IndexCount(draw DrawCall) int {
	if draw.Prepared != nil {
		return len(draw.Prepared.Indices()) / IndexSize
	}

	return len(draw.Indices)
}

func validateInstanceCount(draw DrawCall) error {
	count := max(len(draw.Instances), 1)

	const maximumInstanceBytes = 128 << 20
	if uint64(count) > math.MaxUint32 || count > math.MaxInt/InstanceStride || count > maximumInstanceBytes/InstanceStride {
		return errGeometryCount
	}

	return nil
}
