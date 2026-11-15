package lightmapper

import "math"

type receiverClipVertex struct {
	uv   [2]float64
	bary [3]float64
}
type receiverPixelPiece struct {
	pixel   uint32
	piece   FootprintPiece
	polygon []receiverClipVertex
}

func receiverClipTriangle(index uint32, uv [3]Vec2, width, height uint32) ([]receiverPixelPiece, error) {
	return receiverClipTriangleRows(index, uv, width, height, 0, height)
}

//nolint:varnamelen // x/y are integer texel coordinates.
func receiverClipTriangleRows(index uint32, uv [3]Vec2, width, height, first, last uint32) ([]receiverPixelPiece, error) {
	polygon := make([]receiverClipVertex, 3)

	for i := range 3 {
		for axis := range 2 {
			if !receiverFinite(uv[i][axis]) {
				return nil, errReceiver
			}
		}

		polygon[i].uv = [2]float64{float64(uv[i][0]) * float64(width), float64(uv[i][1]) * float64(height)}
		polygon[i].bary[i] = 1
	}

	minimum, maximum := polygon[0].uv, polygon[0].uv
	for _, vertex := range polygon[1:] {
		for axis := range 2 {
			minimum[axis] = min(minimum[axis], vertex.uv[axis])
			maximum[axis] = max(maximum[axis], vertex.uv[axis])
		}
	}

	if maximum[0] <= 0 || maximum[1] <= 0 || minimum[0] >= float64(width) || minimum[1] >= float64(height) {
		return nil, nil
	}

	xmin := uint32(math.Floor(max(float64(0), minimum[0])))
	ymin := uint32(math.Floor(max(float64(0), minimum[1])))
	xmax := uint32(math.Ceil(min(float64(width), maximum[0])))
	ymax := uint32(math.Ceil(min(float64(height), maximum[1])))
	ymin = max(ymin, first)
	ymax = min(ymax, last)

	var result []receiverPixelPiece

	for y := ymin; y < ymax; y++ {
		for x := xmin; x < xmax; x++ {
			piece, err := receiverClipPixel(index, polygon, x, y, width)
			if err != nil {
				return nil, err
			}

			if piece != nil {
				result = append(result, *piece)
			}
		}
	}

	return result, nil
}

//nolint:varnamelen // t is the affine edge intersection parameter.
func receiverClipPlane(input []receiverClipVertex, axis int, boundary float64, greater bool) []receiverClipVertex {
	if len(input) == 0 {
		return nil
	}

	output := make([]receiverClipVertex, 0, len(input)+1)
	previous := input[len(input)-1]
	inside := func(value float64) bool {
		if greater {
			return value >= boundary
		}

		return value <= boundary
	}

	previousInside := inside(previous.uv[axis])
	for _, current := range input {
		currentInside := inside(current.uv[axis])
		if currentInside != previousInside {
			t := (boundary - previous.uv[axis]) / (current.uv[axis] - previous.uv[axis])

			var vertex receiverClipVertex
			for dimension := range 2 {
				vertex.uv[dimension] = previous.uv[dimension] + t*(current.uv[dimension]-previous.uv[dimension])
			}

			vertex.uv[axis] = boundary
			for dimension := range 3 {
				vertex.bary[dimension] = previous.bary[dimension] + t*(current.bary[dimension]-previous.bary[dimension])
			}

			output = append(output, vertex)
		}

		if currentInside {
			output = append(output, current)
		}

		previous, previousInside = current, currentInside
	}

	return output
}

//nolint:mnd // The triangle-fan determinant is twice its area.
func receiverPolygonArea(polygon []receiverClipVertex) float64 {
	var result float64

	origin := polygon[0].uv
	for i := 1; i+1 < len(polygon); i++ {
		a, b := polygon[i].uv, polygon[i+1].uv
		result += (a[0]-origin[0])*(b[1]-origin[1]) - (a[1]-origin[1])*(b[0]-origin[0])
	}

	return math.Abs(result) * .5
}

//nolint:varnamelen,mnd,nilnil // x/y are integer texel coordinates and covered support requires at least three polygon corners. A nil optional piece denotes exact zero clipped area, rather than invalid geometry.
func receiverClipPixel(index uint32, polygon []receiverClipVertex, x, y, width uint32) (*receiverPixelPiece, error) {
	clipped := polygon
	clipped = receiverClipPlane(clipped, 0, float64(x), true)
	clipped = receiverClipPlane(clipped, 0, float64(x+1), false)
	clipped = receiverClipPlane(clipped, 1, float64(y), true)

	clipped = receiverClipPlane(clipped, 1, float64(y+1), false)
	if len(clipped) < 3 {
		return nil, nil
	}

	area := receiverPolygonArea(clipped)
	if area <= 0 {
		return nil, nil
	}

	if area > 1.00001 || math.IsNaN(area) || math.IsInf(area, 0) {
		return nil, errReceiver
	}

	piece := FootprintPiece{Triangle: index, Area: float32(area), Barycentric: make([]Vec3, len(clipped))}
	if piece.Area == 0 {
		return nil, errReceiver
	}

	for i, vertex := range clipped {
		for axis := range 3 {
			piece.Barycentric[i][axis] = float32(vertex.bary[axis])
		}
	}

	return &receiverPixelPiece{pixel: y*width + x, piece: piece, polygon: clipped}, nil
}
