//go:build !js

package impostorbake

import (
	"encoding/binary"
	"fmt"
	"math"

	"github.com/karty-game/karty-webgpu/internal/meshdata"
	"github.com/karty-game/karty-webgpu/internal/modeldata"
)

type vertex struct {
	position, normal vector
	uv               [2]float64
}

type triangle struct {
	vertices           [3]vertex
	tangent, bitangent vector
	surface            int
}

type surface struct {
	material       modeldata.Material
	albedo, normal *pixels
}

type geometry struct {
	triangles   []triangle
	surfaces    []surface
	center      vector
	radius      float64
	halfExtents vector
	roughness   float32
}

type extent struct{ minimum, maximum vector }

func (bounds *extent) include(position vector) {
	for axis, coordinate := range position {
		bounds.minimum[axis], bounds.maximum[axis] = min(bounds.minimum[axis], coordinate), max(bounds.maximum[axis], coordinate)
	}
}

func prepare(source modeldata.Document) (geometry, error) {
	if source.Source == "" || len(source.Parts) == 0 || len(source.Parts) > modeldata.MaxParts ||
		len(source.Textures) > modeldata.MaxTextures {
		return geometry{}, errBounds
	}

	textures := make([]pixels, len(source.Textures))
	for index, value := range source.Textures {
		decoded, err := decodeTexture(value)
		if err != nil {
			return geometry{}, fmt.Errorf("texture %d: %w", index, err)
		}

		textures[index] = decoded
	}

	result := geometry{roughness: source.Parts[0].Material.Roughness}
	bounds := extent{minimum: vector{math.Inf(1), math.Inf(1), math.Inf(1)}, maximum: vector{math.Inf(-1), math.Inf(-1), math.Inf(-1)}}

	for _, part := range source.Parts {
		if _, err := meshdata.FromOwned(part.Vertices, part.Indices); err != nil {
			return geometry{}, err
		}

		material, err := prepareSurface(part.Material, source.Textures, textures, result.roughness)
		if err != nil {
			return geometry{}, fmt.Errorf("part %s: %w", part.Name, err)
		}

		materialIndex := len(result.surfaces)

		result.surfaces = append(result.surfaces, material)
		if err := appendPart(&result, part, materialIndex, &bounds); err != nil {
			return geometry{}, err
		}
	}

	result.center = scale(add(bounds.minimum, bounds.maximum), half)
	for _, triangle := range result.triangles {
		for _, vertex := range triangle.vertices {
			delta := subtract(vertex.position, result.center)
			result.radius = max(result.radius, math.Sqrt(dot(delta, delta)))
		}
	}
	// The small fixed guard keeps silhouettes away from tile edges before filtering.
	result.radius *= 1.01
	if !finite(result.radius) || !finite(float64(float32(result.radius))) || result.radius <= 1e-6 {
		return geometry{}, errBounds
	}

	result.halfExtents = scale(subtract(bounds.maximum, bounds.minimum), half*1.01)
	for axis := range result.halfExtents {
		result.halfExtents[axis] = max(result.halfExtents[axis], result.radius*1e-4, minimumProjectionSpan)
	}

	return result, nil
}

func appendPart(result *geometry, part modeldata.Part, materialIndex int, bounds *extent) error {
	material := result.surfaces[materialIndex]
	for offset := 0; offset < len(part.Indices); offset += triangleVertices * meshdata.IndexStride {
		value := triangle{surface: materialIndex}

		for corner := range triangleVertices {
			index := binary.LittleEndian.Uint32(part.Indices[offset+corner*meshdata.IndexStride:])
			value.vertices[corner] = readVertex(part.Vertices[int(index)*meshdata.VertexStride:])
			bounds.include(value.vertices[corner].position)
		}

		if err := prepareFrame(&value, material.normal != nil && material.material.NormalStrength != 0); err != nil {
			return err
		}

		result.triangles = append(result.triangles, value)
		if len(result.triangles) > meshdata.MaxIndices/triangleVertices {
			return errBounds
		}
	}

	return nil
}

func readVertex(data []byte) vertex {
	var values [8]float64
	for index := range values {
		values[index] = float64(math.Float32frombits(binary.LittleEndian.Uint32(data[index*4:])))
	}

	return vertex{
		position: vector{values[0], values[1], values[2]},
		normal:   vector{values[3], values[4], values[5]},
		uv:       [2]float64{values[6], values[7]},
	}
}

func prepareSurface(value modeldata.Material, roles []modeldata.Texture, textures []pixels, roughness float32) (surface, error) {
	if !validSurfaceFactors(value, roughness) {
		return surface{}, fmt.Errorf("requires opaque materials, uniform roughness and no AO maps: %w", errUnsupported)
	}

	for _, channel := range value.Color {
		if !finite(float64(channel)) || channel < 0 || channel > 1 {
			return surface{}, errUnsupported
		}
	}

	result := surface{material: value}
	for _, slot := range []struct {
		index  int32
		role   modeldata.TextureRole
		target **pixels
	}{{value.Albedo, modeldata.Albedo, &result.albedo}, {value.Normal, modeldata.Normal, &result.normal}} {
		if slot.index == -1 {
			continue
		}

		if slot.index < 0 || int(slot.index) >= len(textures) || roles[slot.index].Role != slot.role {
			return surface{}, errUnsupported
		}

		*slot.target = &textures[slot.index]
	}

	return result, nil
}

func validSurfaceFactors(value modeldata.Material, roughness float32) bool {
	validRoughness := finite(float64(value.Roughness)) && value.Roughness >= 0 && value.Roughness <= 1
	if !validRoughness || math.Abs(float64(value.Roughness-roughness)) > 1e-6 {
		return false
	}

	return finite(float64(value.NormalStrength)) && value.AO == -1 && value.Color[3] == 1
}

func prepareFrame(value *triangle, normalMap bool) error {
	first, second := subtract(
		value.vertices[1].position,
		value.vertices[0].position,
	), subtract(
		value.vertices[2].position,
		value.vertices[0].position,
	)
	face := unit(cross(first, second))

	for index := range value.vertices {
		if dot(value.vertices[index].normal, value.vertices[index].normal) < geometryEpsilon {
			value.vertices[index].normal = face
		}
	}

	firstU, firstV := value.vertices[1].uv[0]-value.vertices[0].uv[0], value.vertices[1].uv[1]-value.vertices[0].uv[1]
	secondU, secondV := value.vertices[2].uv[0]-value.vertices[0].uv[0], value.vertices[2].uv[1]-value.vertices[0].uv[1]

	determinant := firstU*secondV - secondU*firstV
	if math.Abs(determinant) < geometryEpsilon {
		if normalMap {
			return fmt.Errorf("normal map requires a nondegenerate triangle UV frame: %w", errUnsupported)
		}

		return nil
	}

	value.tangent = scale(subtract(scale(first, secondV), scale(second, firstV)), 1/determinant)
	value.bitangent = scale(subtract(scale(second, firstU), scale(first, secondU)), 1/determinant)

	return nil
}
