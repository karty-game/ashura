//go:build !js

package lightmapbake

import "testing"

func TestWorldAreaCullRetainsExtremeScalePositiveTriangle(t *testing.T) {
	t.Parallel()

	triangle := Triangle{Positions: [3]Vec3{{1e30, 1e30, 0}, {}, {1e-30, 0, 0}}}
	if !hasWorldArea(triangle) {
		t.Fatal("float64 cancellation discarded positive world support")
	}

	triangle.Positions[2] = Vec3{}
	if hasWorldArea(triangle) {
		t.Fatal("repeated vertices acquired world area")
	}
}
