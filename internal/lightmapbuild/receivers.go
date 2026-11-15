//go:build !js

//nolint:varnamelen // Coordinate names follow receiver chart and image equations.
package lightmapbuild

import (
	"context"
	"encoding/binary"
	"fmt"
	"math"

	"github.com/karty-game/karty-webgpu/internal/modeldata"
	"github.com/karty-game/karty-webgpu/internal/xatlas"
)

type fallback struct {
	position, normal   vec
	tangent, bitangent vec
	geometric          vec
	coverage           float32
	set                bool
}

func buildReceivers(
	ctx context.Context,
	parts []modeldata.Part,
	atlas xatlas.Atlas,
	rnm bool,
	geometric []vec,
) ([]byte, []byte, []byte, error) {
	w, h := int(atlas.Width), int(atlas.Height)
	data := make([]byte, w*h*32)

	var frames []byte
	if rnm {
		frames = make([]byte, w*h*16)
	}

	owners := make([]int32, w*h)
	for i := range owners {
		owners[i] = -1
	}

	first := make([]fallback, atlas.Charts)
	for partIndex, part := range parts {
		if err := rasterPart(ctx, part, atlas.Meshes[partIndex], data, frames, geometric, owners, first, w, h); err != nil {
			return nil, nil, nil, err
		}
	}
	// Subpixel charts still need a representative surface sample. It is placed
	// only in a pixel belonging to that chart, never in another chart's gutter.
	seen := make([]bool, atlas.Charts)

	for _, owner := range owners {
		if owner >= 0 {
			seen[owner] = true
		}
	}

	for index, pixel := range atlas.Image {
		if pixel == 0 {
			continue
		}

		chart := int32(pixel & 0x1fffffff)
		if !seen[chart] && first[chart].set && owners[index] < 0 {
			f := first[chart]
			setReceiver(data, index, f.position, f.normal, f.coverage, chart)

			if geometric != nil {
				geometric[index] = f.geometric
			}

			if rnm {
				setFrame(frames, index, f.normal, f.tangent, f.bitangent)
			}

			owners[index] = chart
			seen[chart] = true
		}
	}

	for i, v := range first {
		if v.set && !seen[i] {
			return nil, nil, nil, fmt.Errorf("xatlas chart %d has no receiver pixel: %w", i, errGeometry)
		}
	}

	lookup, err := paddingLookup(ctx, atlas, owners)
	if err != nil {
		return nil, nil, nil, err
	}

	return data, lookup, frames, nil
}

func paddingLookup(ctx context.Context, atlas xatlas.Atlas, owners []int32) ([]byte, error) {
	w, h := int(atlas.Width), int(atlas.Height)
	// Build the output-to-source lookup offline. The compute bake writes these
	// gutters directly, without a runtime image dilation stage.
	lookup := make([]uint32, w*h)
	queue := make([]int, 0, w*h)

	for i := range lookup {
		lookup[i] = math.MaxUint32
		if owners[i] >= 0 {
			lookup[i] = uint32(i)
			queue = append(queue, i)
		}
	}

	// Atlas occupancy bounds each chart's gutter. Thin surface regions may
	// lack texel-center samples for longer than that gutter width, so fill the
	// entire chart support rather than leaving black holes beyond a fixed radius.
	for head := 0; head < len(queue); head++ {
		if head%4096 == 0 {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
		}

		i := queue[head]
		x, y := i%w, i/w

		for _, delta := range [][2]int{{-1, -1}, {0, -1}, {1, -1}, {-1, 0}, {1, 0}, {-1, 1}, {0, 1}, {1, 1}} {
			nx, ny := x+delta[0], y+delta[1]
			if nx < 0 || nx >= w || ny < 0 || ny >= h {
				continue
			}

			j := ny*w + nx
			if lookup[j] != math.MaxUint32 || atlas.Image[j] == 0 || int32(atlas.Image[j]&0x1fffffff) != owners[int(lookup[i])] {
				continue
			}

			lookup[j] = lookup[i]
			queue = append(queue, j)
		}
	}

	out := make([]byte, len(lookup)*4)
	for i, v := range lookup {
		binary.LittleEndian.PutUint32(out[i*4:], v)
	}

	return out, nil
}
