//go:build !js

package decalbuild

import (
	"context"
	"errors"
	"math"

	"github.com/karty-game/ashura/internal/decaldata"
	"github.com/karty-game/ashura/pkg/assetbuild"
	"github.com/karty-game/ashura/pkg/engine"
)

var errReceiverUV = errors.New("receiver needs nonoverlapping, nondegenerate UV triangles inside [0,1]")

type uvPoint [2]float64

type receiverTriangle struct {
	positions [3]decaldata.Vector
	uv        [3]uvPoint
	normal    decaldata.Vector
	area      float64
}

func paintAlbedo(ctx context.Context, image *assetbuild.Image, triangles []Triangle, marks []bakedMark) error {
	covered := make([]bool, image.Width*image.Height)

	for _, input := range triangles {
		triangle, err := prepareTriangle(input)
		if err != nil {
			return err
		}

		if err := triangle.paint(ctx, image, covered, marks); err != nil {
			return err
		}
	}

	return nil
}

func prepareTriangle(input Triangle) (receiverTriangle, error) {
	var result receiverTriangle

	for index, point := range input.UVs {
		if !(point.X >= 0 && point.X <= 1 && point.Y >= 0 && point.Y <= 1) {
			return receiverTriangle{}, errReceiverUV
		}

		for _, coordinate := range vector(input.Positions[index]) {
			if !(math.Abs(float64(coordinate)) <= decaldata.MaxCoordinate) {
				return receiverTriangle{}, errReceiverUV
			}
		}

		result.uv[index] = uvPoint{float64(point.X), float64(point.Y)}
		result.positions[index] = vector(input.Positions[index])
	}

	result.normal = faceNormal(input.Positions)

	result.area = edge(result.uv[0], result.uv[1], result.uv[2])
	if result.area == 0 || result.normal == (decaldata.Vector{}) {
		return receiverTriangle{}, errReceiverUV
	}

	if result.area < 0 {
		result.uv[1], result.uv[2] = result.uv[2], result.uv[1]
		result.positions[1], result.positions[2] = result.positions[2], result.positions[1]
		result.area = -result.area
	}

	return result, nil
}

func faceNormal(points [3]engine.Vector3) decaldata.Vector {
	a, b, c := points[0], points[1], points[2]
	x, y, z := float64(b.X-a.X), float64(b.Y-a.Y), float64(b.Z-a.Z)
	u, v, w := float64(c.X-a.X), float64(c.Y-a.Y), float64(c.Z-a.Z)
	normalX, normalY, normalZ := y*w-z*v, z*u-x*w, x*v-y*u

	length := math.Sqrt(normalX*normalX + normalY*normalY + normalZ*normalZ)
	if length == 0 {
		return decaldata.Vector{}
	}

	return decaldata.Vector{float32(normalX / length), float32(normalY / length), float32(normalZ / length)}
}

func edge(a, b, p uvPoint) float64 { return (b[0]-a[0])*(p[1]-a[1]) - (b[1]-a[1])*(p[0]-a[0]) }

func ownsEdge(a, b uvPoint, value float64) bool {
	return value > 0 || value == 0 && (a[1] < b[1] || a[1] == b[1] && a[0] > b[0])
}

func (triangle receiverTriangle) paint(
	ctx context.Context, image *assetbuild.Image, covered []bool, marks []bakedMark,
) error {
	firstX, lastX := triangle.rasterRange(0, image.Width)

	firstY, lastY := triangle.rasterRange(1, image.Height)
	for row := firstY; row < lastY; row++ {
		if err := ctx.Err(); err != nil {
			return err
		}

		for column := firstX; column < lastX; column++ {
			sample := uvPoint{
				(float64(column) + decaldata.Half) / float64(image.Width),
				(float64(row) + decaldata.Half) / float64(image.Height),
			}

			position, found := triangle.position(sample)
			if !found {
				continue
			}

			index := row*image.Width + column
			if covered[index] {
				return errReceiverUV
			}

			covered[index] = true
			paintPixel(image.Levels[0][index*4:], position, triangle.normal, marks)
		}
	}

	return nil
}

func (triangle receiverTriangle) rasterRange(axis, pixels int) (start, end int) {
	uv := triangle.uv
	minimum := min(uv[0][axis], uv[1][axis], uv[2][axis]) * float64(pixels)
	maximum := max(uv[0][axis], uv[1][axis], uv[2][axis]) * float64(pixels)

	return max(0, int(math.Floor(minimum))), min(pixels, int(math.Ceil(maximum)))
}

func (triangle receiverTriangle) position(sample uvPoint) (decaldata.Vector, bool) {
	a, b, c := triangle.uv[0], triangle.uv[1], triangle.uv[2]

	weights := [3]float64{edge(b, c, sample), edge(c, a, sample), edge(a, b, sample)}
	if !ownsEdge(b, c, weights[0]) || !ownsEdge(c, a, weights[1]) || !ownsEdge(a, b, weights[2]) {
		return decaldata.Vector{}, false
	}

	var position decaldata.Vector
	for axis := range position {
		var coordinate float64
		for corner, weight := range weights {
			coordinate += float64(triangle.positions[corner][axis]) * weight
		}

		position[axis] = float32(coordinate / triangle.area)
	}

	return position, true
}
