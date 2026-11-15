//go:build !js

package modelbuild

import (
	"encoding/binary"
	"fmt"
	"math"

	"github.com/karty-game/karty-webgpu/internal/meshdata"
	"github.com/karty-game/karty-webgpu/internal/modeldata"
)

type attribute struct {
	data                 []byte
	stride, count, width int
}

func (build *compiler) preparePart(source primitive, transform matrix, name string) (modeldata.Part, error) {
	if err := validatePrimitive(source); err != nil {
		return modeldata.Part{}, err
	}

	positions, err := build.attribute(source.Attributes, "POSITION", "VEC3", 0)
	if err != nil || positions.count == 0 {
		return modeldata.Part{}, fmt.Errorf("POSITION: %w", errFormat)
	}

	normals, err := build.attribute(source.Attributes, "NORMAL", "VEC3", positions.count)
	if err != nil {
		return modeldata.Part{}, err
	}

	uvs, err := build.attribute(source.Attributes, "TEXCOORD_0", "VEC2", positions.count)
	if err != nil {
		return modeldata.Part{}, err
	}

	indices, err := build.primitiveIndices(source.Indices, positions.count)
	if err != nil {
		return modeldata.Part{}, err
	}

	if determinant(transform) < 0 {
		for offset := 0; offset < len(indices); offset += 3 {
			indices[offset+1], indices[offset+2] = indices[offset+2], indices[offset+1]
		}
	}

	value, err := build.partMaterial(source.Material, uvs.count)
	if err != nil {
		return modeldata.Part{}, err
	}

	part := modeldata.Part{Name: name, Material: value}

	vertices := positions.count
	if normals.count == 0 {
		vertices = len(indices)
	}

	if err := build.reserveGeometry(vertices, len(indices)); err != nil {
		return part, err
	}

	if normals.count == 0 {
		part.Vertices, part.Indices = flatVertices(positions, uvs, indices, transform)
	} else {
		part.Vertices, part.Indices = smoothVertices(positions, normals, uvs, indices, transform)
	}

	return part, nil
}

func (build *compiler) attribute(attributes map[string]int, semantic, kind string, count int) (attribute, error) {
	index, present := attributes[semantic]
	if !present {
		return attribute{}, nil
	}

	data, stride, err := accessorBytes(build.document, build.binary, index)
	if err != nil {
		return attribute{}, err
	}

	source := build.document.Accessors[index]
	if source.ComponentType != floatComponent || source.Type != kind || source.Count > meshdata.MaxVertices ||
		count != 0 && source.Count != count {
		return attribute{}, fmt.Errorf("%s accessor must have matching float32 %s elements: %w", semantic, kind, errUnsupported)
	}

	if semantic == "POSITION" && (len(source.Min) != 3 || len(source.Max) != 3) {
		return attribute{}, fmt.Errorf("POSITION min/max: %w", errFormat)
	}

	return attribute{data: data, stride: stride, count: source.Count, width: attributeWidth(kind)}, nil
}

//nolint:gosec // Vertex/index counts are bounded positive values before integer casts.
func (build *compiler) primitiveIndices(index *int, vertices int) ([]uint32, error) {
	if index == nil {
		if vertices%3 != 0 {
			return nil, errFormat
		}

		indices := make([]uint32, vertices)
		for value := range indices {
			indices[value] = uint32(value)
		}

		return indices, nil
	}

	data, stride, err := accessorBytes(build.document, build.binary, *index)
	if err != nil {
		return nil, err
	}

	source := build.document.Accessors[*index]
	if source.Type != scalarType || source.ComponentType == floatComponent || source.Count%3 != 0 ||
		stride != componentSize(source.ComponentType) {
		return nil, errFormat
	}

	indices := make([]uint32, source.Count)
	for offset := range indices {
		indexData := data[offset*stride:]

		switch source.ComponentType {
		case unsignedByte:
			indices[offset] = uint32(indexData[0])
		case unsignedShort:
			indices[offset] = uint32(binary.LittleEndian.Uint16(indexData))
		case unsignedInt:
			indices[offset] = binary.LittleEndian.Uint32(indexData)
		}

		if uint64(indices[offset]) >= uint64(vertices) {
			return nil, errFormat
		}
	}

	return indices, nil
}

func smoothVertices(positions, normals, uvs attribute, indices []uint32, transform matrix) ([]byte, []byte) {
	vertices := make([]byte, 0, positions.count*meshdata.VertexStride)
	for index := range positions.count {
		vertices = appendVertex(
			vertices,
			position(transform, positions.vector(index)),
			normal(transform, normals.vector(index)),
			uvs.vector(index),
		)
	}

	encoded := make([]byte, 0, len(indices)*meshdata.IndexStride)
	for _, index := range indices {
		encoded = binary.LittleEndian.AppendUint32(encoded, index)
	}

	return vertices, encoded
}

func flatVertices(positions, uvs attribute, indices []uint32, transform matrix) ([]byte, []byte) {
	vertices := make([]byte, 0, len(indices)*meshdata.VertexStride)

	encoded := make([]byte, 0, len(indices)*meshdata.IndexStride)
	for offset := 0; offset < len(indices); offset += 3 {
		points := [3]vector{position(transform, positions.vector(int(indices[offset]))),
			position(transform, positions.vector(int(indices[offset+1]))), position(transform, positions.vector(int(indices[offset+2])))}

		direction := normalized(cross(subtract(points[1], points[0]), subtract(points[2], points[0])))
		for corner := range points {
			vertices = appendVertex(vertices, points[corner], direction, uvs.vector(int(indices[offset+corner])))
			encoded = binary.LittleEndian.AppendUint32(encoded, uint32(offset+corner))
		}
	}

	return vertices, encoded
}

func (value attribute) vector(index int) vector {
	if value.count == 0 {
		return vector{}
	}

	data := value.data[index*value.stride:]
	width := value.width

	var result vector
	for component := range width {
		result[component] = float64(math.Float32frombits(binary.LittleEndian.Uint32(data[component*wordSize:])))
	}

	return result
}

func appendVertex(data []byte, point, direction, uv vector) []byte {
	for _, value := range []float64{point[0], point[1], point[2], direction[0], direction[1], direction[2], uv[0], uv[1]} {
		data = binary.LittleEndian.AppendUint32(data, math.Float32bits(float32(value)))
	}

	return data
}

func validatePrimitive(source primitive) error {
	if source.Mode != nil && *source.Mode != triangles || source.Targets != nil {
		return fmt.Errorf("only static TRIANGLES are supported: %w", errUnsupported)
	}

	for semantic := range source.Attributes {
		if semantic != "POSITION" && semantic != "NORMAL" && semantic != "TEXCOORD_0" {
			return fmt.Errorf("attribute %s: %w", semantic, errUnsupported)
		}
	}

	return nil
}

func (build *compiler) partMaterial(index *int, uvs int) (modeldata.Material, error) {
	if index == nil {
		return modeldata.Material{}, fmt.Errorf(
			"implicit default material has unsupported metallicFactor=1; author material metallicFactor=0: %w",
			errUnsupported,
		)
	}

	if *index < 0 || *index >= len(build.materials) {
		return modeldata.Material{}, errFormat
	}

	value := build.materials[*index]
	if uvs == 0 && (value.Albedo >= 0 || value.Normal >= 0 || value.AO >= 0) {
		return value, fmt.Errorf("textured primitive requires TEXCOORD_0: %w", errFormat)
	}

	return value, nil
}

func (build *compiler) reserveGeometry(vertices, indices int) error {
	if vertices > meshdata.MaxVertices-build.vertices || indices > meshdata.MaxIndices-build.indices {
		return errBounds
	}

	if (build.vertices+vertices)*meshdata.VertexStride+(build.indices+indices)*meshdata.IndexStride > modeldata.MaxBytes-build.textureBytes {
		return errBounds
	}

	build.vertices += vertices
	build.indices += indices

	return nil
}
