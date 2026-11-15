//go:build !js

package levelbuild

import (
	"fmt"
	"math"
	"strings"

	"github.com/karty-game/karty-webgpu/internal/meshdata"
	"github.com/karty-game/karty-webgpu/internal/modeldata"
)

func normalize(options Options) (Options, error) {
	if options.Size == ([3]float32{}) {
		options.Size = [3]float32{32, 32, 32}
	}

	if options.MaxChunks == 0 {
		options.MaxChunks = MaxChunks
	}

	if options.MaxTriangles == 0 {
		options.MaxTriangles = MaxTriangles
	}

	if options.MaxChunks > MaxChunks || options.MaxTriangles > MaxTriangles {
		return Options{}, ErrGeometry
	}

	for axis := range 3 {
		if !finite(float64(options.Size[axis])) || options.Size[axis] <= 0 || !finite(float64(options.Origin[axis])) {
			return Options{}, ErrGeometry
		}
	}

	return options, nil
}

func validatePlacement(placement Placement) error {
	if !validIdentity(placement.ID) || !validIdentity(placement.Space) || len(placement.Group) > 4096 {
		return ErrGeometry
	}

	if len(placement.Model.Parts) == 0 || len(placement.Model.Parts) > modeldata.MaxParts ||
		len(placement.Model.Textures) > modeldata.MaxTextures {
		return ErrGeometry
	}

	for _, image := range placement.Model.Textures {
		if image.Role > modeldata.Occlusion || len(image.Data) == 0 || len(image.Data) > modeldata.MaxBytes {
			return ErrGeometry
		}
	}

	vertices, indices := 0, 0

	for _, part := range placement.Model.Parts {
		if _, err := meshdata.FromOwned(part.Vertices, part.Indices); err != nil {
			return fmt.Errorf("%s primitive %s: %w", placement.ID, part.Name, err)
		}

		vertices += len(part.Vertices) / meshdata.VertexStride

		indices += len(part.Indices) / meshdata.IndexStride
		if vertices > meshdata.MaxVertices || indices > meshdata.MaxIndices || len(part.Name) > 4096 ||
			!validMaterial(part.Material, placement.Model.Textures) {
			return ErrGeometry
		}
	}

	for name, group := range placement.PartGroups {
		if len(name) > 4096 || len(group) > 4096 {
			return ErrGeometry
		}
	}

	return nil
}

func validMaterial(material modeldata.Material, textures []modeldata.Texture) bool {
	for _, channel := range material.Color {
		if !finite(float64(channel)) || channel < 0 || channel > 1 {
			return false
		}
	}

	if material.Color[3] != 1 || !finite(float64(material.Roughness)) || material.Roughness < 0 || material.Roughness > 1 ||
		!finite(
			float64(material.NormalStrength),
		) || !finite(float64(material.AOStrength)) || material.AOStrength < 0 || material.AOStrength > 1 {
		return false
	}

	for role, index := range []int32{material.Albedo, material.Normal, material.AO} {
		if index == -1 {
			continue
		}

		if index < 0 || int64(index) >= int64(len(textures)) || textures[index].Role != modeldata.TextureRole(role) {
			return false
		}
	}

	return true
}

func validIdentity(value string) bool {
	return value != "" && len(value) <= 4096 && !strings.ContainsAny(value, "\x00\r\n")
}

func finite(value float64) bool { return !math.IsNaN(value) && !math.IsInf(value, 0) }
