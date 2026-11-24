//go:build !js

package impostorbake

import (
	"context"
	"math"
	"sort"

	"github.com/karty-game/ashura/internal/impostordata"
)

const maximumLayerFragments = 1 << 20

type layerFragments struct {
	pixels       [][]layerHit
	count, limit int
}

func (fragments *layerFragments) add(index int, hit layerHit) error {
	if fragments.count >= fragments.limit {
		return fmtLayerError("all-intersection working set overflow", float64(fragments.count))
	}

	fragments.pixels[index] = append(fragments.pixels[index], hit)
	fragments.count++

	return nil
}

type layerHit struct {
	depth    float64
	triangle int
	weights  [3]float64
	delta    int
}
type layerInterval struct{ front, back layerHit }

func layeredRaster(ctx context.Context, geometry geometry, view impostordata.View, resolution int) ([][]layerHit, error) {
	pixels := make([][]layerHit, resolution*resolution)
	fragments := layerFragments{pixels: pixels, limit: maximumLayerFragments}

	buffer := rasterizer{size: resolution, geometry: geometry, view: view}
	for triangleID, value := range geometry.triangles {
		if triangleID%256 == 0 {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
		}

		facing := dot(
			cross(
				subtract(value.vertices[1].position, value.vertices[0].position),
				subtract(value.vertices[2].position, value.vertices[0].position),
			),
			doubleVector(view.Outward),
		)
		if facing == 0 {
			continue
		}

		delta := 1
		if facing < 0 {
			delta = -1
		}

		points := [3]projected{
			buffer.project(value.vertices[0].position),
			buffer.project(value.vertices[1].position),
			buffer.project(value.vertices[2].position),
		}
		corners := [3]int{0, 1, 2}

		area := edge(points[0], points[1], points[2].column, points[2].row)
		if math.Abs(area) < geometryEpsilon {
			continue
		}

		if area < 0 {
			points[1], points[2] = points[2], points[1]
			corners[1], corners[2] = corners[2], corners[1]
			area = -area
		}

		if err := rasterLayerTriangle(&fragments, points, corners, area, triangleID, delta, resolution); err != nil {
			return nil, err
		}
	}

	return pixels, nil
}

func rasterLayerTriangle(
	fragments *layerFragments,
	points [3]projected,
	corners [3]int,
	area float64,
	triangleID, delta, resolution int,
) error {
	left, right := max(
		0,
		int(math.Floor(min(points[0].column, points[1].column, points[2].column))),
	), min(
		resolution-1,
		int(math.Ceil(max(points[0].column, points[1].column, points[2].column))),
	)

	top, bottom := max(
		0,
		int(math.Floor(min(points[0].row, points[1].row, points[2].row))),
	), min(
		resolution-1,
		int(math.Ceil(max(points[0].row, points[1].row, points[2].row))),
	)
	for row := top; row <= bottom; row++ {
		for column := left; column <= right; column++ {
			x, y := float64(column)+half, float64(row)+half

			values := [3]float64{edge(points[1], points[2], x, y), edge(points[2], points[0], x, y), edge(points[0], points[1], x, y)}
			if !layerInside(values[0], points[1], points[2]) || !layerInside(values[1], points[2], points[0]) ||
				!layerInside(values[2], points[0], points[1]) {
				continue
			}

			hit := layerHit{triangle: triangleID, delta: delta}
			for corner, value := range values {
				hit.weights[corners[corner]] = value / area
				hit.depth += value * points[corner].depth / area
			}

			index := row*resolution + column
			if err := fragments.add(index, hit); err != nil {
				return err
			}
		}
	}

	return nil
}
func layerInside(value float64, first, second projected) bool {
	topLeft := second.row < first.row || second.row == first.row && second.column > first.column

	return value > 1e-9 || math.Abs(value) <= 1e-9 && topLeft
}

func unionLayers(hits []layerHit, radius float64, maximum int, report *Report) ([]layerInterval, error) {
	sort.Slice(hits, func(i, j int) bool { return hits[i].depth > hits[j].depth })

	winding := 0

	var (
		front         layerHit
		intervals     []layerInterval
		negativeStart float64
	)

	for start := 0; start < len(hits); {
		end := start + 1

		delta := hits[start].delta
		for end < len(hits) && math.Abs(hits[end].depth-hits[start].depth) <= radius*1e-6 {
			delta += hits[end].delta
			end++
		}

		before := winding

		winding += delta
		if before > 0 && winding < 0 || before < 0 && winding > 0 {
			return nil, fmtLayerError("inconsistent winding sign jump", hits[start].depth)
		}

		if before == 0 && winding < 0 {
			negativeStart = hits[start].depth
		}

		if before < 0 {
			excursion := negativeStart - hits[start].depth
			if excursion > radius*1e-4 {
				return nil, fmtLayerError("negative winding", excursion)
			}

			if winding == 0 {
				report.ToleratedNegativePockets++
				report.MaximumNegativePocketDepth = max(report.MaximumNegativePocketDepth, excursion)
			}
		}

		if before == 0 && winding > 0 {
			front = boundaryHit(hits[start:end], 1)
		}

		if before > 0 && winding == 0 {
			intervals = append(intervals, layerInterval{front: front, back: boundaryHit(hits[start:end], -1)})

			report.MaximumIntervals = max(report.MaximumIntervals, len(intervals))
			if len(intervals) > maximum {
				return nil, fmtLayerError("occupied interval overflow", float64(len(intervals)))
			}
		}

		start = end
	}

	if winding != 0 {
		return nil, fmtLayerError("unclosed winding", float64(winding))
	}

	return intervals, nil
}
func boundaryHit(hits []layerHit, sign int) layerHit {
	selected := hits[0]

	found := false
	for _, hit := range hits {
		if hit.delta == sign && (!found || hit.triangle > selected.triangle) {
			selected = hit
			found = true
		}
	}

	return selected
}
