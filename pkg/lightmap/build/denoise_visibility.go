//go:build !js

//nolint:mnd // Fixed BVH/triangle byte layout, three axes and short surface segments.
package lightmapbuild

import (
	"context"
	"encoding/binary"
	"math"

	"github.com/karty-game/ashura/internal/lightmapdata"
)

const (
	denoiseRight = 1
	denoiseDown  = 2
)

// Two segment tests per covered texel establish local surface connectivity.
// Wider filter taps follow these edges rather than tracing every candidate.
func denoiseLinks(ctx context.Context, document *lightmapdata.Document) ([]byte, error) {
	width, height := int(document.Width), int(document.Height)

	links := make([]byte, width*height)
	for pixelY := range height {
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		for pixelX := range width {
			pixel := pixelY*width + pixelX

			center := denoiseGuideAt(document, pixel)
			if center.coverage == 0 {
				continue
			}

			if pixelX+1 < width && denoiseVisibleNeighbor(document, center, pixel+1) {
				links[pixel] |= denoiseRight
			}

			if pixelY+1 < height && denoiseVisibleNeighbor(document, center, pixel+width) {
				links[pixel] |= denoiseDown
			}
		}
	}

	return links, nil
}

func denoiseVisibleNeighbor(document *lightmapdata.Document, center denoiseGuide, pixel int) bool {
	neighbor := denoiseGuideAt(document, pixel)
	if _, weight := denoiseGeometry(center, neighbor); weight == 0 {
		return false
	}

	var origin, direction [3]float32
	for axis := range 3 {
		origin[axis] = center.position[axis] + center.normal[axis]*.015
		direction[axis] = neighbor.position[axis] + neighbor.normal[axis]*.015 - origin[axis]
	}

	return !denoiseOccluded(document, origin, direction)
}

func denoiseConnected(links []byte, width, pixelX, pixelY, neighborX, neighborY int) bool {
	for pixelX < neighborX {
		if links[pixelY*width+pixelX]&denoiseRight == 0 {
			return false
		}

		pixelX++
	}

	for pixelX > neighborX {
		pixelX--
		if links[pixelY*width+pixelX]&denoiseRight == 0 {
			return false
		}
	}

	for pixelY < neighborY {
		if links[pixelY*width+pixelX]&denoiseDown == 0 {
			return false
		}

		pixelY++
	}

	for pixelY > neighborY {
		pixelY--
		if links[pixelY*width+pixelX]&denoiseDown == 0 {
			return false
		}
	}

	return true
}

func denoiseOccluded(document *lightmapdata.Document, origin, direction [3]float32) bool {
	var stack [64]int

	length := 1
	for length > 0 {
		length--

		node := document.Nodes[stack[length]*lightmapdata.NodeStride:]
		if !denoiseBox(node, origin, direction) {
			continue
		}

		first := int(binary.LittleEndian.Uint32(node[12:]))

		count := int(binary.LittleEndian.Uint32(node[28:]))
		if count > 0 {
			for triangle := first; triangle < first+count; triangle++ {
				if denoiseTriangle(document.Triangles[triangle*lightmapdata.TriangleStride:], origin, direction) {
					return true
				}
			}

			continue
		}

		if length+2 > len(stack) {
			return true
		}

		stack[length], stack[length+1] = first, first+1
		length += 2
	}

	return false
}

func denoiseBox(node []byte, origin, direction [3]float32) bool {
	near, far := float32(0), float32(1)

	for axis := range 3 {
		minimum, maximum := denoiseNumber(node[axis*4:]), denoiseNumber(node[16+axis*4:])
		if math.Abs(float64(direction[axis])) < 1e-10 {
			if origin[axis] < minimum || origin[axis] > maximum {
				return false
			}

			continue
		}

		first, second := (minimum-origin[axis])/direction[axis], (maximum-origin[axis])/direction[axis]

		near, far = max(near, min(first, second)), min(far, max(first, second))
		if near > far {
			return false
		}
	}

	return true
}

func denoiseTriangle(triangle []byte, origin, direction [3]float32) bool {
	var first, second, delta [3]float32

	for axis := range 3 {
		base := denoiseNumber(triangle[axis*4:])
		first[axis] = denoiseNumber(triangle[16+axis*4:]) - base
		second[axis] = denoiseNumber(triangle[32+axis*4:]) - base
		delta[axis] = origin[axis] - base
	}

	perpendicular := denoiseCross(direction, second)

	determinant := denoiseDot(first, perpendicular)
	if math.Abs(float64(determinant)) < 1e-12 {
		return false
	}

	barycentricU := denoiseDot(delta, perpendicular) / determinant
	if barycentricU < -1e-6 || barycentricU > 1.000001 {
		return false
	}

	other := denoiseCross(delta, first)

	barycentricV := denoiseDot(direction, other) / determinant
	if barycentricV < -1e-6 || barycentricU+barycentricV > 1.000001 {
		return false
	}

	distance := denoiseDot(second, other) / determinant

	return distance > 1e-5 && distance < .99999
}

func denoiseCross(first, second [3]float32) [3]float32 {
	return [3]float32{
		first[1]*second[2] - first[2]*second[1],
		first[2]*second[0] - first[0]*second[2],
		first[0]*second[1] - first[1]*second[0],
	}
}

func denoiseDot(first, second [3]float32) float32 {
	return first[0]*second[0] + first[1]*second[1] + first[2]*second[2]
}
