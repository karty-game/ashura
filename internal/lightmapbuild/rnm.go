//go:build !js

//nolint:mnd,varnamelen // UV derivatives and ABI offsets follow the renderer's tangent frame.
package lightmapbuild

import (
	"fmt"
	"math"

	"github.com/karty-game/karty-webgpu/internal/ktx2data"
	"github.com/karty-game/karty-webgpu/internal/modeldata"
	"github.com/karty-game/karty-webgpu/internal/texturedata"
)

// triangleTangents differentiates world positions against material UV0, never
// against the independently rotated and packed lightmap charts.
func triangleTangents(points [3]vec, uv [3][2]float32) (vec, vec) {
	first, second := sub(points[1], points[0]), sub(points[2], points[0])
	u1, v1 := uv[1][0]-uv[0][0], uv[1][1]-uv[0][1]
	u2, v2 := uv[2][0]-uv[0][0], uv[2][1]-uv[0][1]

	determinant := u1*v2 - u2*v1
	if math.Abs(float64(determinant)) < 1e-8 {
		return vec{}, vec{}
	}

	var tangent, bitangent vec
	for axis := range 3 {
		tangent[axis] = (first[axis]*v2 - second[axis]*v1) / determinant
		bitangent[axis] = (second[axis]*u1 - first[axis]*u2) / determinant
	}

	return tangent, bitangent
}

func setFrame(frames []byte, index int, normal, rawTangent, rawBitangent vec) {
	tangent := sub(rawTangent, scale(normal, dot(normal, rawTangent)))

	fallback := lengthSquared(tangent) < 1e-16
	if fallback {
		axis := vec{0, 1, 0}
		if math.Abs(float64(normal[1])) > .9 {
			axis = vec{1, 0, 0}
		}

		tangent = cross(axis, normal)
	}

	tangent = normalize(tangent)

	handedness := float32(1)
	if !fallback && dot(cross(normal, tangent), rawBitangent) < 0 {
		handedness = -1
	}

	for axis := range 3 {
		putFloat(frames[index*16+axis*4:], tangent[axis])
	}

	putFloat(frames[index*16+12:], handedness)
}
func dot(a, b vec) float32            { return a[0]*b[0] + a[1]*b[1] + a[2]*b[2] }
func scale(v vec, scalar float32) vec { return vec{v[0] * scalar, v[1] * scalar, v[2] * scalar} }

// buildSurfaces uses a per-material mean reflectance approximation for diffuse
// bounces. Source textures are averaged in linear light, with the material color
// factor applied once. This coarse radiosity input intentionally excludes SSBump
// self-occlusion, which belongs to the final receiver's material response.
func buildSurfaces(triangles []triangle, model modeldata.Document) ([]byte, error) {
	means := make(map[int32]vec)

	reflectance := make([]vec, len(model.Parts))
	for index, part := range model.Parts {
		color := vec{1, 1, 1}

		if part.Material.Albedo >= 0 {
			var exists bool

			color, exists = means[part.Material.Albedo]
			if !exists {
				data, err := ktx2data.Decode(model.Textures[part.Material.Albedo].Data)
				if err != nil {
					return nil, fmt.Errorf("lightmap reflectance texture: %w", err)
				}

				color = averageReflectance(data)
				means[part.Material.Albedo] = color
			}
		}

		for axis := range 3 {
			reflectance[index][axis] = max(0, min(.95, color[axis]*part.Material.Color[axis]))
		}
	}

	surfaces := make([]byte, len(triangles)*16)
	for index, triangle := range triangles {
		for axis := range 3 {
			putFloat(surfaces[index*16+axis*4:], reflectance[triangle.part][axis])
		}

		if model.Parts[triangle.part].Material.DoubleSided {
			putFloat(surfaces[index*16+12:], 1)
		}
	}

	return surfaces, nil
}
func averageReflectance(data texturedata.Data) vec {
	// Average full-resolution texels rather than an sRGB-coded final mip.
	pixels := data.Levels[0]

	var sums [3]float64

	for offset := 0; offset < len(pixels); offset += 4 {
		for axis := range 3 {
			value := float64(pixels[offset+axis]) / 255
			if data.ColorSpace == texturedata.SRGB {
				if value <= .04045 {
					value /= 12.92
				} else {
					value = math.Pow((value+.055)/1.055, 2.4)
				}
			}

			sums[axis] += value
		}
	}

	count := float64(len(pixels) / 4)

	return vec{float32(sums[0] / count), float32(sums[1] / count), float32(sums[2] / count)}
}
