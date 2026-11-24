//go:build !js

// Package collisionbuild prepares static collision meshes in Ashura asset recipes.
package collisionbuild

import (
	"cmp"
	"errors"
	"fmt"
	"math"
	"slices"

	"github.com/karty-game/ashura/pkg/collision"
	"github.com/karty-game/ashura/pkg/engine"
)

const (
	maxTriangles      = 1 << 18
	leafSize          = 8
	partitionChildren = 2
)

var errGeometry = errors.New("invalid or oversized collision geometry")

type primitive struct {
	triangle collision.Triangle
	bounds   collision.Bounds
	center   [3]float64
}

type tree struct {
	nodes     []collision.NodeV1
	triangles []collision.Triangle
}

func prepare(triangles []collision.Triangle) (tree, error) {
	if len(triangles) > maxTriangles {
		return tree{}, errGeometry
	}

	primitives := make([]primitive, 0, len(triangles))
	for _, triangle := range triangles {
		if !finite(triangle.A) || !finite(triangle.B) || !finite(triangle.C) {
			return tree{}, errGeometry
		}

		if degenerate(triangle) {
			continue
		}

		bounds := union(pointBounds(triangle.A), union(pointBounds(triangle.B), pointBounds(triangle.C)))
		center := [3]float64{
			midpoint(bounds.Min.X, bounds.Max.X), midpoint(bounds.Min.Y, bounds.Max.Y), midpoint(bounds.Min.Z, bounds.Max.Z),
		}
		primitives = append(primitives, primitive{triangle: triangle, bounds: bounds, center: center})
	}

	if len(primitives) == 0 {
		return tree{}, fmt.Errorf("no nondegenerate triangles: %w", errGeometry)
	}

	result := tree{triangles: make([]collision.Triangle, 0, len(primitives))}
	result.partition(primitives)

	return result, nil
}

func (result *tree) partition(primitives []primitive) {
	index := len(result.nodes)
	result.nodes = append(result.nodes, collision.NodeV1{})
	bounds := primitives[0].bounds

	for _, item := range primitives[1:] {
		bounds = union(bounds, item.bounds)
	}

	node := collision.NodeV1{Bounds: bounds}
	if len(primitives) <= leafSize {
		//nolint:gosec // The offline input cap bounds the triangle range to 2^18.
		node.First, node.Count = uint32(len(result.triangles)), uint32(len(primitives))
		for _, item := range primitives {
			result.triangles = append(result.triangles, item.triangle)
		}
	} else {
		axis := longestAxis(bounds)

		slices.SortStableFunc(primitives, func(a, b primitive) int {
			return cmp.Compare(a.center[axis], b.center[axis])
		})

		mid := len(primitives) / partitionChildren
		result.partition(primitives[:mid])
		result.partition(primitives[mid:])
	}

	//nolint:gosec // A binary tree has fewer than twice the bounded input count.
	node.Skip = uint32(len(result.nodes))
	result.nodes[index] = node
}

func pointBounds(point engine.Vector3) collision.Bounds {
	return collision.Bounds{Min: point, Max: point}
}

func union(a, b collision.Bounds) collision.Bounds {
	return collision.Bounds{
		Min: engine.Vector3{X: min(a.Min.X, b.Min.X), Y: min(a.Min.Y, b.Min.Y), Z: min(a.Min.Z, b.Min.Z)},
		Max: engine.Vector3{X: max(a.Max.X, b.Max.X), Y: max(a.Max.Y, b.Max.Y), Z: max(a.Max.Z, b.Max.Z)},
	}
}

func longestAxis(bounds collision.Bounds) int {
	xSpan := float64(bounds.Max.X) - float64(bounds.Min.X)
	ySpan := float64(bounds.Max.Y) - float64(bounds.Min.Y)

	zSpan := float64(bounds.Max.Z) - float64(bounds.Min.Z)
	switch {
	case xSpan >= ySpan && xSpan >= zSpan:
		return 0
	case ySpan >= zSpan:
		return 1
	default:
		const zAxis = 2

		return zAxis
	}
}

func finite(v engine.Vector3) bool {
	return finiteComponent(v.X) && finiteComponent(v.Y) && finiteComponent(v.Z)
}

func finiteComponent(value float32) bool {
	return !math.IsNaN(float64(value)) && !math.IsInf(float64(value), 0)
}

func degenerate(triangle collision.Triangle) bool {
	a, b := difference(triangle.B, triangle.A), difference(triangle.C, triangle.A)

	return a[1]*b[2] == a[2]*b[1] && a[2]*b[0] == a[0]*b[2] && a[0]*b[1] == a[1]*b[0]
}

func difference(a, b engine.Vector3) [3]float64 {
	return [3]float64{float64(a.X) - float64(b.X), float64(a.Y) - float64(b.Y), float64(a.Z) - float64(b.Z)}
}

func midpoint(a, b float32) float64 {
	const half = 0.5

	return (float64(a) + float64(b)) * half
}
