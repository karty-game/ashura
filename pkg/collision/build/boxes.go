//go:build !js

package collisionbuild

import (
	"fmt"
	"slices"

	"github.com/karty-game/ashura/pkg/assetbuild"
	"github.com/karty-game/ashura/pkg/collision"
	"github.com/karty-game/ashura/pkg/engine"
)

const boxTriangles = 12

type boxPublisher struct {
	name  string
	boxes []collision.Bounds
}

// Boxes publishes an offline static mesh for world-space blockout geometry.
// name is the exported function name in the generated colliders package.
func Boxes(name string, boxes ...collision.Bounds) assetbuild.Plugin {
	return boxPublisher{name: name, boxes: slices.Clone(boxes)}
}

func (boxPublisher) Name() string { return "box-collision" }

func (plugin boxPublisher) Plan(plan *assetbuild.Plan) error {
	return publish(plan, plugin.name, meshAction(plugin.name, "box-collision/"+plugin.name, plugin.boxes, nil,
		func(_ *assetbuild.Context) ([]collision.Triangle, error) { return boxFaces(plugin.boxes) }))
}

func boxFaces(boxes []collision.Bounds) ([]collision.Triangle, error) {
	if len(boxes) == 0 || len(boxes) > maxTriangles/boxTriangles {
		return nil, errGeometry
	}

	triangles := make([]collision.Triangle, 0, len(boxes)*boxTriangles)
	for _, box := range boxes {
		minimum, maximum := box.Min, box.Max
		if !finite(minimum) || !finite(maximum) || minimum.X >= maximum.X || minimum.Y >= maximum.Y || minimum.Z >= maximum.Z {
			return nil, fmt.Errorf("invalid collision box: %w", errGeometry)
		}

		corners := [8]engine.Vector3{
			minimum,
			{X: maximum.X, Y: minimum.Y, Z: minimum.Z},
			{X: maximum.X, Y: maximum.Y, Z: minimum.Z},
			{X: minimum.X, Y: maximum.Y, Z: minimum.Z},
			{
				X: minimum.X,
				Y: minimum.Y,
				Z: maximum.Z,
			},
			{X: maximum.X, Y: minimum.Y, Z: maximum.Z},
			maximum,
			{X: minimum.X, Y: maximum.Y, Z: maximum.Z},
		}

		faces := [6][4]int{{0, 1, 2, 3}, {4, 7, 6, 5}, {0, 3, 7, 4}, {1, 5, 6, 2}, {0, 4, 5, 1}, {3, 2, 6, 7}}
		for _, face := range faces {
			a, b, c, d := corners[face[0]], corners[face[1]], corners[face[2]], corners[face[3]]
			triangles = append(triangles, collision.Triangle{A: a, B: b, C: c}, collision.Triangle{A: a, B: c, C: d})
		}
	}

	return triangles, nil
}
