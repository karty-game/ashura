//go:build !js

package impostorbake

import (
	"context"
	"math"

	"github.com/karty-game/ashura/internal/impostordata"
)

const poleThreshold = .9999

func octahedralView(column, row, grid int) impostordata.View {
	horizontal, vertical := float64(column)*2/float64(grid-1)-1, float64(row)*2/float64(grid-1)-1

	upward := 1 - math.Abs(horizontal) - math.Abs(vertical)
	if upward < 0 {
		oldHorizontal, oldVertical := horizontal, vertical
		horizontal = (1 - math.Abs(oldVertical)) * signNonZero(oldHorizontal)
		vertical = (1 - math.Abs(oldHorizontal)) * signNonZero(oldVertical)
	}

	outward := unit(vector{horizontal, upward, vertical})

	right := vector{1, 0, 0}
	if math.Abs(outward[1]) < poleThreshold {
		right = unit(cross(vector{0, 1, 0}, outward))
	}

	up := unit(cross(outward, right))

	return impostordata.View{Right: floatVector(right), Up: floatVector(up), Outward: floatVector(outward)}
}

func signNonZero(value float64) float64 {
	if value < 0 {
		return -1
	}

	return 1
}

func captureOctahedral(ctx context.Context, geometry geometry, options Options) ([][]byte, [][]byte, []impostordata.View, error) {
	colors, normals := atlasLevels(options)

	views := make([]impostordata.View, 0, options.FrameGrid*options.FrameGrid)
	for row := range options.FrameGrid {
		for column := range options.FrameGrid {
			if err := ctx.Err(); err != nil {
				return nil, nil, nil, err
			}

			view := octahedralView(column, row, options.FrameGrid)

			color, normal := raster(ctx, geometry, view, options.Resolution)
			if err := ctx.Err(); err != nil {
				return nil, nil, nil, err
			}

			views = append(views, view)

			packView(colors, normals, color, normal, column, row, options)
		}
	}

	return colors, normals, views, nil
}
