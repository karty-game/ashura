//go:build !js

package render3dbuild

import (
	"math"
	"testing"
)

func TestCompositionRoundTripsAndRejectsShear(t *testing.T) {
	t.Parallel()

	for _, rotation := range [][3]float64{{0, 0, 0}, {0.3, -0.7, 1.1}, {0, math.Pi / 2, 0}, {0, -math.Pi / 2, 0}} {
		original := local([3]float64{3, 4, 5}, rotation, [3]float64{2, 3, 4})

		position, resultRotation, scale, err := decompose(original)
		if err != nil {
			t.Fatal(err)
		}

		roundTrip := local(position, resultRotation, scale)
		for index, value := range original {
			if math.Abs(value-roundTrip[index]) > 1e-6 {
				t.Fatalf("rotation %v component %d: %g vs %g", rotation, index, value, roundTrip[index])
			}
		}
	}

	sheared := multiply(
		local([3]float64{}, [3]float64{}, [3]float64{2, 1, 1}),
		local([3]float64{}, [3]float64{0, 0, 0.5}, [3]float64{1, 1, 1}),
	)
	if _, _, _, err := decompose(sheared); err == nil {
		t.Fatal("shear silently approximated")
	}

	parent := local([3]float64{10, 0, 0}, [3]float64{0, math.Pi / 2, 0}, [3]float64{2, 2, 2})
	child := local([3]float64{1, 0, 0}, [3]float64{}, [3]float64{1, 1, 1})

	position, _, _, err := decompose(multiply(parent, child))
	if err != nil || math.Abs(position[0]-10) > 1e-6 || math.Abs(position[2]+2) > 1e-6 {
		t.Fatalf("nested placement %v: %v", position, err)
	}
}

func TestWorldTransformUsesFactoryCompositionAndRejectsShear(t *testing.T) {
	t.Parallel()

	parent := map[string]map[string]any{"spatial.transform3d": {
		"position": []any{float64(10), float64(0), float64(0)},
		"rotation": []any{float64(0), math.Pi / 2, float64(0)},
		"scale":    []any{float64(2), float64(2), float64(2)},
	}}
	child := map[string]map[string]any{"spatial.transform3d": {
		"position": []any{float64(1), float64(0), float64(0)},
	}}

	world, err := WorldTransform([]map[string]map[string]any{parent}, child)
	if err != nil || math.Abs(float64(world[12])-10) > 1e-6 || math.Abs(float64(world[14])+2) > 1e-6 || world[15] != 1 {
		t.Fatalf("resolved and generated placement transforms differ: %v, %v", world, err)
	}

	parent["spatial.transform3d"]["scale"] = []any{float64(2), float64(1), float64(1)}

	child["spatial.transform3d"]["rotation"] = []any{float64(0), float64(0), .5}
	if _, err = WorldTransform([]map[string]map[string]any{parent}, child); err == nil {
		t.Fatal("bake transform silently admitted shear rejected by scene factories")
	}
}
