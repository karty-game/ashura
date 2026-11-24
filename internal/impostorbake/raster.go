//go:build !js

package impostorbake

import (
	"context"
	"math"

	"github.com/karty-game/ashura/internal/impostordata"
)

const supersample = 2

type projected struct {
	column, row, depth float64
}

type rasterizer struct {
	size          int
	depth         []float64
	color, normal []byte
	geometry      geometry
	view          impostordata.View
}

func raster(
	ctx context.Context,
	source geometry,
	view impostordata.View,
	resolution int,
) ([]byte, []byte) {
	size := resolution * supersample

	buffer := rasterizer{
		size:     size,
		depth:    make([]float64, size*size),
		color:    make([]byte, size*size*channels),
		normal:   make([]byte, size*size*channels),
		geometry: source,
		view:     view,
	}
	for index := range buffer.depth {
		buffer.depth[index] = math.Inf(-1)
	}

	for index, triangle := range source.triangles {
		if index%256 == 0 && ctx.Err() != nil {
			return nil, nil
		}

		buffer.triangle(triangle)
	}

	color, normal := reduce(buffer.color, buffer.normal, size)

	return color, normal
}

func (buffer *rasterizer) project(position vector) projected {
	delta := subtract(position, buffer.geometry.center)

	return projected{column: (half + half*dot(delta, doubleVector(buffer.view.Right))/buffer.geometry.radius) * float64(buffer.size),
		row:   (half - half*dot(delta, doubleVector(buffer.view.Up))/buffer.geometry.radius) * float64(buffer.size),
		depth: dot(delta, doubleVector(buffer.view.Outward))}
}

func (buffer *rasterizer) triangle(value triangle) {
	material := buffer.geometry.surfaces[value.surface]

	face := cross(
		subtract(value.vertices[1].position, value.vertices[0].position),
		subtract(value.vertices[2].position, value.vertices[0].position),
	)

	frontFacing := dot(face, doubleVector(buffer.view.Outward)) > 0
	if !material.material.DoubleSided && !frontFacing {
		return
	}

	points := [3]projected{
		buffer.project(value.vertices[0].position),
		buffer.project(value.vertices[1].position),
		buffer.project(value.vertices[2].position),
	}

	area := edge(points[0], points[1], points[2].column, points[2].row)
	if math.Abs(area) < geometryEpsilon {
		return
	}

	left := max(0, int(math.Floor(min(points[0].column, points[1].column, points[2].column))))
	right := min(buffer.size-1, int(math.Ceil(max(points[0].column, points[1].column, points[2].column))))
	top := max(0, int(math.Floor(min(points[0].row, points[1].row, points[2].row))))

	bottom := min(buffer.size-1, int(math.Ceil(max(points[0].row, points[1].row, points[2].row))))
	for row := top; row <= bottom; row++ {
		for column := left; column <= right; column++ {
			weights := [3]float64{edge(points[1], points[2], float64(column)+.5, float64(row)+.5) / area,
				edge(points[2], points[0], float64(column)+.5, float64(row)+.5) / area, 0}

			weights[2] = 1 - weights[0] - weights[1]
			if min(weights[0], weights[1], weights[2]) < -1e-9 {
				continue
			}

			depth := weights[0]*points[0].depth + weights[1]*points[1].depth + weights[2]*points[2].depth

			index := row*buffer.size + column
			if depth <= buffer.depth[index] {
				continue
			}

			buffer.depth[index] = depth
			buffer.fragment(value, material, weights, index, depth, frontFacing)
		}
	}
}

func edge(first, second projected, column, row float64) float64 {
	return (second.column-first.column)*(row-first.row) - (second.row-first.row)*(column-first.column)
}

func (buffer *rasterizer) fragment(value triangle, material surface, weights [3]float64, index int, depth float64, frontFacing bool) {
	albedo, normal := sampleSurface(value, material, weights)

	// Match Standard's front_facing path: flip the complete mapped direction after
	// tangent-frame conversion, preserving the correct tangent-XY hemisphere.
	if !frontFacing {
		normal = scale(normal, -1)
	}

	for channel := range 3 {
		buffer.color[index*channels+channel] = quantize(toSRGB(albedo[channel]))
		buffer.normal[index*channels+channel] = quantize(normal[channel]*.5 + .5)
	}

	buffer.color[index*channels+3] = byteMaximum
	buffer.normal[index*channels+3] = quantize(.5 + .5*depth/buffer.geometry.radius)
}

func sampleSurface(value triangle, material surface, weights [3]float64) (vector, vector) {
	var (
		normal      vector
		coordinates [2]float64
	)

	for corner, weight := range weights {
		normal = add(normal, scale(value.vertices[corner].normal, weight))
		coordinates[0] += weight * value.vertices[corner].uv[0]
		coordinates[1] += weight * value.vertices[corner].uv[1]
	}

	normal = unit(normal)
	if material.normal != nil && material.material.NormalStrength != 0 {
		sampled := material.normal.sample(coordinates)
		sampled = vector{
			(sampled[0]*2 - 1) * float64(material.material.NormalStrength),
			(sampled[1]*2 - 1) * float64(material.material.NormalStrength),
			sampled[2]*2 - 1,
		}
		tangent := unit(subtract(value.tangent, scale(normal, dot(value.tangent, normal))))

		bitangent := cross(normal, tangent)
		if dot(bitangent, value.bitangent) < 0 {
			bitangent = scale(bitangent, -1)
		}

		normal = unit(add(add(scale(tangent, sampled[0]), scale(bitangent, sampled[1])), scale(normal, sampled[2])))
	}

	albedo := material.albedo.sample(coordinates)
	for channel := range 3 {
		albedo[channel] *= float64(material.material.Color[channel])
	}

	return albedo, normal
}
