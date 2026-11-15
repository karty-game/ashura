//go:build !js

package lightmapbuild

import (
	"fmt"
	"math"

	"github.com/karty-game/karty-webgpu/internal/lightmapdata"
	"github.com/karty-game/karty-webgpu/internal/xatlas"
)

// Preflight every page before tracing any of them. Pixel coverage is bounded by
// the complete page; each receiver retains at most eight links. A physical patch
// is also bounded by its source triangle's grid-cell AABB and sidedness, so we
// avoid the much looser eight-unique-patches-per-receiver estimate when possible.
func (preparer *Preparer) preflightPages(pages []xatlas.Page, options PageOptions) error {
	if !options.PrecomputeTransport {
		return nil
	}

	for _, page := range pages {
		bytes := preparer.pageByteUpperBound(page)
		if bytes > lightmapdata.MaxTransportBytes {
			return fmt.Errorf(
				"prepared page conservative upper bound%d bytes including UV/header exceeds%d: %w",
				bytes,
				lightmapdata.MaxTransportBytes,
				errTransportBudget,
			)
		}
	}

	return nil
}

func (preparer *Preparer) pageByteUpperBound(page xatlas.Page) uint64 {
	pixels := uint64(page.Width) * uint64(page.Height)
	links := pixels * lightmapdata.MaxGITransfers
	patches := min(links, preparer.patchUpperBound)
	bytes := uint64(len(preparer.nodes) + len(preparer.positions) + len(preparer.surfaces))

	const pixelIndexBytes = 4

	bytes += pixels * (lightmapdata.ReceiverStride + lightmapdata.FrameStride + pixelIndexBytes + pixelIndexBytes)
	bytes += pixels*lightmapdata.GIReceiverStride + links*lightmapdata.GITransferStride + patches*lightmapdata.GIPatchStride

	const transportHeaderBytes = 144

	bytes += transportHeaderBytes
	for _, mesh := range page.Meshes {
		bytes += pixelIndexBytes + uint64(len(mesh.Vertices))*lightmapdata.UVStride
	}

	return bytes
}

// Only the bound is needed, so stop once all possible portable-buffer space
// would be occupied by patches. Saturation avoids loops or integer overflow for
// exceptionally large source triangles; such pages still fail/split explicitly.
func transportPatchUpperBound(triangles []triangle, surfaces []byte) uint64 {
	const maximum = uint64(lightmapdata.MaxTransportBytes/lightmapdata.GIPatchStride + 1)

	var magnitude float64

	for _, triangle := range triangles {
		for _, point := range triangle.points {
			for _, value := range point {
				magnitude = max(magnitude, math.Abs(float64(value)))
			}
		}
	}

	const reconstructionMargin = 4e-6

	margin := reconstructionMargin * (1 + magnitude)

	var total uint64

	for index, triangle := range triangles {
		count := uint64(1)

		for axis := range 3 {
			low := min(float64(triangle.points[0][axis]), float64(triangle.points[1][axis]), float64(triangle.points[2][axis]))
			high := max(float64(triangle.points[0][axis]), float64(triangle.points[1][axis]), float64(triangle.points[2][axis]))
			// Intersection tolerances and float32 hit reconstruction can put a sample
			// slightly outside its source triangle. Include that conservative margin.
			first, last := math.Floor((low-margin)/transportCellSize), math.Floor((high+margin)/transportCellSize)
			span := last - first + 1
			// The existing key retains IEEE signed zero; account for both bit patterns.
			if first <= 0 && last >= 0 {
				span++
			}

			if span >= float64(maximum) || float64(count)*span >= float64(maximum) {
				return maximum
			}

			count *= uint64(span)
		}

		if readFloat(surfaces[index*lightmapdata.SurfaceStride+12:]) > 0 {
			count *= 2
		}

		total += count
		if total >= maximum {
			return maximum
		}
	}

	return total
}
