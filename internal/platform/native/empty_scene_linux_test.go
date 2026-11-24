//go:build linux

package native

import (
	"testing"

	"github.com/karty-game/ashura/internal/render"
	wgpu "github.com/karty-game/ashura/internal/webgpu"
)

func TestEmptySceneRetiresPreviousGeometryAndInstances(t *testing.T) {
	t.Parallel()

	driver := &Driver{instances: map[uint32]*gpuInstances{1: {}}}
	draw := render.DrawCall{Vertices: make([]render.Vertex, 3), Indices: []uint32{0, 1, 2}}
	created := 0

	create := func(_, _, _ []byte) (gpuGeometry, error) {
		created++

		// Zero handles exercise retirement without creating any GPU resources.
		return gpuGeometry{vertices: &wgpu.Buffer{}, indices: &wgpu.Buffer{}}, nil
	}
	if _, err := driver.geometry.Resolve(draw, create); err != nil {
		t.Fatal(err)
	}

	// The previous 3D frame retained the geometry and reset its live marker.
	driver.geometry.Prune(gpuGeometry.release)
	// A nil encoder deliberately stops at the backend boundary. Empty-frame
	// retirement must happen even when recording its clear pass fails.
	if err := driver.drawScenePass(nil, nil, render.Frame{}); err == nil {
		t.Fatal("accepted a nil command encoder")
	}

	if len(driver.instances) != 0 {
		t.Fatal("empty frame retained previous 3D instances")
	}

	if _, err := driver.geometry.Resolve(draw, create); err != nil {
		t.Fatal(err)
	}

	if created != 2 {
		t.Fatal("empty frame retained previous 3D geometry")
	}

	driver.geometry.Clear(gpuGeometry.release)
}
