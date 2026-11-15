//go:build !js

//nolint:mnd // sRGB transfer constants and the admitted interleaved model ABI are fixed numeric contracts.
package lightmapbake

import (
	"encoding/binary"
	"fmt"
	"math"

	"github.com/karty-game/karty-webgpu/internal/ktx2data"
	"github.com/karty-game/karty-webgpu/internal/meshdata"
	"github.com/karty-game/karty-webgpu/internal/modeldata"
	"github.com/karty-game/karty-webgpu/pkg/texture"
)

// SceneFromModel converts an existing admitted prepared-model payload offline.
// No source geometry or texture decoder is linked into the runtime loader.
func SceneFromModel(data []byte, space string, metresPerUnit float32) (Scene, error) {
	scene, err := sceneFromModel(data, space, metresPerUnit)
	if err != nil {
		return Scene{}, err
	}

	visible := scene.Triangles[:0]
	for _, triangle := range scene.Triangles {
		if hasWorldArea(triangle) {
			visible = append(visible, triangle)
		}
	}

	scene.Triangles = visible

	return scene, nil
}

// sceneFromModel retains triangle order for the exact secondary-UV binding.
//
//nolint:gosec // Model admission bounds dimensions, part counts and indexed vertex conversions.
func sceneFromModel(data []byte, space string, metresPerUnit float32) (Scene, error) {
	doc, err := modeldata.Decode(data)
	if err != nil {
		return Scene{}, err
	}

	triangles, err := modelTriangles(doc)
	if err != nil {
		return Scene{}, err
	}

	scene := Scene{ID: doc.Source, Space: space, MetresPerUnit: metresPerUnit}
	usedTextures := make([]bool, len(doc.Textures))

	textureIndices := make([]uint32, len(doc.Textures))
	for _, part := range doc.Parts {
		if part.Material.Albedo >= 0 {
			usedTextures[part.Material.Albedo] = true
		}
	}

	for index, source := range doc.Textures {
		if !usedTextures[index] {
			continue
		}

		textureIndices[index] = uint32(len(scene.Textures)) + 1

		image, err := decodeTransportTexture(source.Data)
		if err != nil {
			return Scene{}, err
		}

		scene.Textures = append(scene.Textures, image)
	}

	for _, part := range doc.Parts {
		mat := Material{ID: reflectanceBinding(doc, part.Material) + "/" + part.Name,
			Reflectance: Vec3{part.Material.Color[0], part.Material.Color[1], part.Material.Color[2]},
			DoubleSided: part.Material.DoubleSided,
		}
		if part.Material.Albedo >= 0 {
			mat.Texture = textureIndices[part.Material.Albedo]
		}

		scene.Materials = append(scene.Materials, mat)
	}

	scene.Triangles = triangles

	return scene, nil
}

// modelTriangles consumes an admitted model and retains its exact part/index
// order. Receiver-only preparation does not need decoded transport textures.
//
// Complete mesh admission bounds all vertex indices and part counts before extraction.
func modelTriangles(doc modeldata.Document) ([]Triangle, error) {
	count := 0

	for _, part := range doc.Parts {
		if _, err := meshdata.FromOwned(part.Vertices, part.Indices); err != nil {
			return nil, err
		}

		count += len(part.Indices) / 12
	}

	var triangles []Triangle
	if count > 0 {
		triangles = make([]Triangle, 0, count)
	}

	for partIndex, part := range doc.Parts {
		for offset := 0; offset < len(part.Indices); offset += 12 {
			tri := Triangle{ID: uint64(len(triangles)) + 1, Material: uint32(partIndex)}

			for corner := range 3 {
				index := int(binary.LittleEndian.Uint32(part.Indices[offset+corner*4:]))

				vertex := part.Vertices[index*32:]
				for axis := range 3 {
					tri.Positions[corner][axis] = readNumber(vertex[axis*4:])
					tri.Normals[corner][axis] = readNumber(vertex[12+axis*4:])
				}

				tri.UV[corner] = Vec2{readNumber(vertex[24:]), readNumber(vertex[28:])}
			}

			triangles = append(triangles, tri)
		}
	}

	return triangles, nil
}

func readNumber(data []byte) float32 { return math.Float32frombits(binary.LittleEndian.Uint32(data)) }

//nolint:gosec // G115: admitted KTX2 dimensions fit the bounded transport texture fields.
func decodeTransportTexture(data []byte) (Texture, error) {
	image, err := ktx2data.Decode(data)
	if err != nil {
		return Texture{}, err
	}

	if image.Format != texture.RGBA8 {
		return Texture{}, fmt.Errorf("bake model textures must be prepared uncompressed inputs: %w", ErrUnsupported)
	}

	pixels := make([]Vec3, image.Width*image.Height)
	for pixel := range pixels {
		for axis := range 3 {
			value := float64(image.Levels[0][pixel*4+axis]) / 255
			if image.ColorSpace == texture.SRGB {
				if value <= .04045 {
					value /= 12.92
				} else {
					value = math.Pow((value+.055)/1.055, 2.4)
				}
			}

			pixels[pixel][axis] = float32(value)
		}
	}

	return Texture{Width: uint32(image.Width), Height: uint32(image.Height), Pixels: pixels}, nil
}
