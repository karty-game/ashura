//go:build !js

package impostorbake

import (
	"encoding/binary"
	"fmt"
	"math"

	"github.com/karty-game/karty-webgpu/internal/impostordata"
)

func fmtLayerError(reason string, value float64) error {
	return fmt.Errorf("layered %s (%g): %w", reason, value, errUnsupported)
}

func packLayeredAxis(
	geometry geometry,
	pixels [][]layerHit,
	color, numeric []byte,
	layout impostordata.LayerLayout,
	options Options,
	axis int,
	report *Report,
) error {
	for index, hits := range pixels {
		intervals, err := unionLayers(hits, geometry.radius, options.MaxIntervals, report)
		if err != nil {
			return fmt.Errorf("pixel(%d,%d): %w", index%options.Resolution, index/options.Resolution, err)
		}

		if len(intervals) == 0 {
			continue
		}

		column, row := index%options.Resolution, index/options.Resolution

		for slot, interval := range intervals {
			front, back := layerDepth(interval.front.depth, geometry.radius), layerDepth(interval.back.depth, geometry.radius)
			if front == 0 || back == 0 || front < back {
				return fmtLayerError("unguarded or inverted depth", interval.front.depth)
			}

			pair := layerTileIndex(layout, options, axis*options.MaxIntervals+slot, column, row, int(layout.PairBaseRow))
			binary.LittleEndian.PutUint16(numeric[pair:], front)
			binary.LittleEndian.PutUint16(numeric[pair+2:], back)

			for side, hit := range []layerHit{interval.front, interval.back} {
				target := layerTileIndex(
					layout,
					options,
					axis*(intervalBoundaries*options.MaxIntervals)+slot*intervalBoundaries+side,
					column,
					row,
					0,
				)
				value := geometry.triangles[hit.triangle]

				albedo, normal := sampleSurface(value, geometry.surfaces[value.surface], hit.weights)
				for channel := range 3 {
					color[target+channel] = quantize(toSRGB(albedo[channel]))
				}

				color[target+3] = 255
				numeric[target], numeric[target+1] = octNormal(normal)
				depth := layerDepth(hit.depth, geometry.radius)
				binary.LittleEndian.PutUint16(numeric[target+2:], depth)
			}
		}

		first, last := intervals[0].front.depth, intervals[len(intervals)-1].back.depth
		leaf := (options.Resolution*options.Resolution-1)/quadtreeDivisor + index
		target := hierarchyIndex(layout, axis, leaf)
		//nolint:gosec // unionLayers bounds the interval count to validated MaxIntervals<=16.
		numeric[target], numeric[target+1], numeric[target+2], numeric[target+3] = byte(
			math.Floor(float64(layerDepth(last, geometry.radius))*255/65535),
		), byte(
			math.Ceil(float64(layerDepth(first, geometry.radius))*255/65535),
		), byte(
			len(intervals),
		), 255
	}

	buildLayerHierarchy(numeric, layout, axis, options.Resolution)

	return nil
}
func layerDepth(depth, radius float64) uint16 {
	return uint16(math.Round(min(1, max(0, .5+.5*depth/radius)) * 65535))
}
func octNormal(normal vector) (byte, byte) {
	divisor := math.Abs(normal[0]) + math.Abs(normal[1]) + math.Abs(normal[2])
	normal = scale(normal, 1/divisor)

	x, y := normal[0], normal[1]
	if normal[2] < 0 {
		x, y = (1-math.Abs(y))*signNonZero(x), (1-math.Abs(x))*signNonZero(y)
	}

	return quantize(x*.5 + .5), quantize(y*.5 + .5)
}
func layerTileIndex(layout impostordata.LayerLayout, options Options, tile, column, row, baseRow int) int {
	x := (tile%impostordata.LayeredColumns)*int(layout.CellSize) + options.Padding + column
	y := baseRow + (tile/impostordata.LayeredColumns)*int(layout.CellSize) + options.Padding + row

	return (y*int(layout.Width) + x) * channels
}
func hierarchyIndex(layout impostordata.LayerLayout, axis, node int) int {
	return (int(layout.HierarchyBaseRow)*int(layout.Width) + axis*int(layout.NodesPerAxis) + node) * channels
}
func buildLayerHierarchy(numeric []byte, layout impostordata.LayerLayout, axis, resolution int) {
	for side := resolution / quadtreeSide; side >= 1; side /= quadtreeSide {
		offset, childOffset := (side*side-1)/quadtreeDivisor, (quadtreeChildren*side*side-1)/quadtreeDivisor
		for row := range side {
			for column := range side {
				target := hierarchyIndex(layout, axis, offset+row*side+column)

				for dy := range 2 {
					for dx := range 2 {
						child := hierarchyIndex(layout, axis, childOffset+(row*quadtreeSide+dy)*(side*quadtreeSide)+column*quadtreeSide+dx)
						if numeric[child+3] == 0 {
							continue
						}

						if numeric[target+3] == 0 {
							copy(numeric[target:target+4], numeric[child:child+4])

							continue
						}

						numeric[target], numeric[target+1], numeric[target+2] = min(
							numeric[target],
							numeric[child],
						), max(
							numeric[target+1],
							numeric[child+1],
						), max(
							numeric[target+2],
							numeric[child+2],
						)
					}
				}
			}
		}
	}
}
