package render_test

import (
	"slices"
	"testing"

	"github.com/karty-game/ashura/internal/meshdata"
	"github.com/karty-game/ashura/internal/render"
)

func boundedDraw(id uint32, x, y, z float32) render.DrawCall {
	return render.DrawCall{
		ID: id, Transform: [16]float32{0: 1, 5: 1, 10: 1, 12: x, 13: y, 14: z, 15: 1},
		Bounds: meshdata.Bounds{Minimum: [3]float32{-.5, -.5, -.5}, Maximum: [3]float32{.5, .5, .5}, Valid: true},
	}
}

func TestOpaqueOrderKeepsBarriersAndStableDepthTies(t *testing.T) {
	t.Parallel()

	frame := render.Frame{
		ViewProjection: [16]float32{0: 1, 5: 1, 10: 1, 15: 1},
		Draws: []render.DrawCall{
			boundedDraw(1, 0, 0, 8), boundedDraw(2, 0, 0, 1), boundedDraw(3, 0, 0, 1),
			boundedDraw(4, 0, 0, 2), boundedDraw(5, 0, 0, 9), boundedDraw(6, 0, 0, 1),
		},
	}
	frame.Draws[3].AlphaMask = true

	var cache render.OpaqueOrder
	for range 2 {
		if order := cache.Prepare(frame); !slices.Equal(order, []int{1, 2, 0, 3, 5, 4}) {
			t.Fatal("opaque depth order with fixed cutout", order)
		}
	}

	frame.Draws[0].Transform[14] = 0
	if order := cache.Prepare(frame); !slices.Equal(order, []int{0, 1, 2, 3, 5, 4}) {
		t.Fatal("transform change did not invalidate the order", order)
	}

	frame.ViewProjection[10] = -1
	if order := cache.Prepare(frame); !slices.Equal(order, []int{1, 2, 0, 3, 4, 5}) {
		t.Fatal("view change did not invalidate the order", order)
	}
}

func TestPointShadowBoundsKeepSeamsInstancesAndUnboundedCasters(t *testing.T) {
	t.Parallel()

	light := render.Light{GroundColor: [3]float32{10}}
	for _, test := range []struct {
		draw render.DrawCall
		mask uint8
	}{
		{boundedDraw(1, 4, 0, 0), 1},
		{boundedDraw(1, 4, 4, 0), 1 | 1<<2},
		{boundedDraw(1, 0, 0, -4), 1 << 5},
		{boundedDraw(1, 0, 0, -20), 0},
		{boundedDraw(1, 0, 0, 0), 63},
		{render.DrawCall{}, 63},
	} {
		if got := render.PointShadowFaceMask(test.draw, light); got != test.mask {
			t.Fatal("unexpected mask for", test.draw.Transform, got, test.mask)
		}
	}

	draw := boundedDraw(1, 4, 0, 0)
	mirrored := boundedDraw(2, 0, 0, -4)
	mirrored.Transform[0] = -3

	draw.Instances = []render.InstanceTransform{{Model: draw.Transform}, {Model: mirrored.Transform}}
	if got := render.PointShadowFaceMask(draw, light); got != 1|1<<5 {
		t.Fatal("instance union lost a cube face", got)
	}
}
