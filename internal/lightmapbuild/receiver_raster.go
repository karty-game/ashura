//go:build !js

//nolint:mnd,varnamelen // Numeric offsets encode the GPU ABI; coordinate names follow barycentric/BVH equations.
package lightmapbuild

import (
	"context"
	"fmt"
	"math"

	"github.com/karty-game/ashura/internal/modeldata"
	"github.com/karty-game/ashura/internal/xatlas"
)

type receiverTriangle struct {
	positions, normals [3]vec
	uv                 [3][2]float64
	chart              int32
	coverage           float32
	tangent, bitangent vec
	geometric          vec
}

func rasterPart(
	ctx context.Context,
	part modeldata.Part,
	mesh xatlas.Mesh,
	data, frames []byte,
	geometric []vec,
	owners []int32,
	first []fallback,
	width, height int,
) error {
	coverage := float32(1)
	if part.Material.DoubleSided {
		coverage = 2
	}

	for offset := 0; offset < len(mesh.Indices); offset += 3 {
		if offset%1536 == 0 {
			if err := ctx.Err(); err != nil {
				return err
			}
		}

		var materialUV [3][2]float32

		tri := receiverTriangle{chart: mesh.Vertices[mesh.Indices[offset]].Chart, coverage: coverage}
		for corner := range 3 {
			index := mesh.Indices[offset+corner]
			tri.positions[corner] = vertex(part.Vertices, index, 0)
			tri.normals[corner] = vertex(part.Vertices, index, 12)
			materialUV[corner] = [2]float32{readFloat(part.Vertices[int(index)*32+24:]), readFloat(part.Vertices[int(index)*32+28:])}

			value := mesh.Vertices[index]
			if value.Chart != tri.chart {
				return fmt.Errorf("triangle crosses xatlas charts: %w", errGeometry)
			}

			tri.uv[corner] = [2]float64{float64(value.UV[0]), float64(value.UV[1])}
		}

		if lengthSquared(cross(sub(tri.positions[1], tri.positions[0]), sub(tri.positions[2], tri.positions[0]))) < 1e-18 {
			continue
		}

		tri.geometric = normalize(cross(sub(tri.positions[1], tri.positions[0]), sub(tri.positions[2], tri.positions[0])))

		if tri.chart < 0 {
			return fmt.Errorf("nondegenerate triangle omitted by xatlas: %w", errGeometry)
		}

		if len(frames) != 0 {
			tri.tangent, tri.bitangent = triangleTangents(tri.positions, materialUV)
		}

		if !first[tri.chart].set {
			first[tri.chart] = fallback{
				position: interpolate(tri.positions, 1.0/3, 1.0/3),
				tangent:  tri.tangent, bitangent: tri.bitangent,
				normal:    normalize(interpolate(tri.normals, 1.0/3, 1.0/3)),
				coverage:  coverage,
				geometric: tri.geometric,
				set:       true,
			}
		}

		if err := rasterTriangle(tri, data, frames, geometric, owners, width, height); err != nil {
			return err
		}
	}

	return nil
}
func rasterTriangle(tri receiverTriangle, data, frames []byte, geometric []vec, owners []int32, width, height int) error {
	uv := tri.uv

	den := (uv[1][1]-uv[2][1])*(uv[0][0]-uv[2][0]) + (uv[2][0]-uv[1][0])*(uv[0][1]-uv[2][1])
	if math.Abs(den) < 1e-15 {
		return nil
	}

	xmin := max(0, int(math.Ceil(min(uv[0][0], uv[1][0], uv[2][0])-.5)))
	xmax := min(width-1, int(math.Floor(max(uv[0][0], uv[1][0], uv[2][0])-.5)))
	ymin := max(0, int(math.Ceil(min(uv[0][1], uv[1][1], uv[2][1])-.5)))

	ymax := min(height-1, int(math.Floor(max(uv[0][1], uv[1][1], uv[2][1])-.5)))
	for y := ymin; y <= ymax; y++ {
		for x := xmin; x <= xmax; x++ {
			u := ((uv[1][1]-uv[2][1])*(float64(x)+.5-uv[2][0]) + (uv[2][0]-uv[1][0])*(float64(y)+.5-uv[2][1])) / den

			v := ((uv[2][1]-uv[0][1])*(float64(x)+.5-uv[2][0]) + (uv[0][0]-uv[2][0])*(float64(y)+.5-uv[2][1])) / den
			if u < -1e-7 || v < -1e-7 || u+v > 1+1e-7 {
				continue
			}

			index := y*width + x
			if owners[index] >= 0 {
				if owners[index] != tri.chart {
					return fmt.Errorf("overlapping xatlas chart receivers: %w", errGeometry)
				}

				continue
			}

			normal := normalize(interpolate(tri.normals, u, v))
			setReceiver(data, index, interpolate(tri.positions, u, v), normal, tri.coverage, tri.chart)

			if geometric != nil {
				geometric[index] = tri.geometric
			}

			if len(frames) != 0 {
				setFrame(frames, index, normal, tri.tangent, tri.bitangent)
			}

			owners[index] = tri.chart
		}
	}

	return nil
}

func interpolate(v [3]vec, u, w float64) vec {
	var out vec
	for i := range 3 {
		out[i] = float32(float64(v[0][i])*u + float64(v[1][i])*w + float64(v[2][i])*(1-u-w))
	}

	return out
}
func setReceiver(data []byte, index int, p, n vec, coverage float32, chart int32) {
	pixel := data[index*32:]
	for i := range 3 {
		putFloat(pixel[i*4:], p[i])
		putFloat(pixel[16+i*4:], n[i])
	}

	putFloat(pixel[12:], coverage)
	putFloat(pixel[28:], float32(chart+1))
}
