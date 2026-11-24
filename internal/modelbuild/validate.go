//go:build !js

package modelbuild

import (
	"fmt"

	"github.com/karty-game/ashura/internal/meshdata"
	"github.com/karty-game/ashura/internal/modeldata"
)

const scalarType = "SCALAR"
const shortSize, vectorWidth = 2, 3
const visited = 2

func validateDocument(document gltf, binaryData []byte) error {
	if err := validateProfile(document); err != nil {
		return err
	}

	if err := validateBuffers(document, binaryData); err != nil {
		return err
	}

	for index := range document.Accessors {
		if _, _, err := accessorBytes(document, binaryData, index); err != nil {
			return fmt.Errorf("accessor %d: %w", index, err)
		}
	}

	return validateNodes(document)
}

//nolint:gosec // Accessor counts/strides are validated positive before unsigned range arithmetic.
func accessorBytes(document gltf, binaryData []byte, index int) ([]byte, int, error) {
	if index < 0 || index >= len(document.Accessors) {
		return nil, 0, errFormat
	}

	value := document.Accessors[index]
	if err := validateAccessor(value, len(document.BufferViews)); err != nil {
		return nil, 0, err
	}

	component, width := componentSize(value.ComponentType), attributeWidth(value.Type)
	if component == 0 || width == 0 || value.Type != scalarType && value.ComponentType != floatComponent ||
		value.ByteOffset%component != 0 {
		return nil, 0, errUnsupported
	}

	view := document.BufferViews[*value.BufferView]

	stride := view.ByteStride
	if stride == 0 {
		stride = component * width
	}

	if stride < component*width || value.ByteOffset > view.ByteLength || (view.ByteOffset+value.ByteOffset)%component != 0 ||
		uint64(value.Count-1)*uint64(stride)+uint64(component*width) > uint64(view.ByteLength-value.ByteOffset) {
		return nil, 0, errFormat
	}

	start := view.ByteOffset + value.ByteOffset

	return binaryData[start : view.ByteOffset+view.ByteLength], stride, nil
}

func componentSize(value int) int {
	switch value {
	case unsignedByte:
		return 1
	case unsignedShort:
		return shortSize
	case unsignedInt, floatComponent:
		return wordSize
	default:
		return 0
	}
}

func attributeWidth(value string) int {
	switch value {
	case scalarType:
		return 1
	case "VEC2":
		return shortSize
	case "VEC3":
		return vectorWidth
	default:
		return 0
	}
}

func validateNodes(document gltf) error {
	parents := make([]int, len(document.Nodes))
	for index, value := range document.Nodes {
		if value.Camera != nil || value.Skin != nil || len(value.Weights) != 0 ||
			value.Mesh != nil && (*value.Mesh < 0 || *value.Mesh >= len(document.Meshes)) {
			return fmt.Errorf("node %d camera/skin/weights/mesh: %w", index, errUnsupported)
		}

		if _, err := localMatrix(value); err != nil {
			return fmt.Errorf("node %d transform: %w", index, err)
		}

		for _, child := range value.Children {
			if child < 0 || child >= len(document.Nodes) || child == index {
				return errFormat
			}

			parents[child]++
			if parents[child] > 1 {
				return fmt.Errorf("node %d has multiple parents: %w", child, errFormat)
			}
		}
	}

	states := make([]uint8, len(document.Nodes))

	heights := make([]int, len(document.Nodes))
	for index := range document.Nodes {
		if err := visitNode(document, states, heights, index, 0); err != nil {
			return err
		}
	}

	return validateRoots(document, parents)
}

func visitNode(document gltf, states []uint8, heights []int, index, depth int) error {
	if depth >= maxJSONDepth || states[index] == 1 {
		return fmt.Errorf("cyclic or excessive node hierarchy: %w", errFormat)
	}

	if states[index] == visited {
		return nil
	}

	states[index] = 1

	heights[index] = 1
	for _, child := range document.Nodes[index].Children {
		if err := visitNode(document, states, heights, child, depth+1); err != nil {
			return err
		}

		heights[index] = max(heights[index], heights[child]+1)
		if heights[index] > maxJSONDepth {
			return errBounds
		}
	}

	states[index] = visited

	return nil
}

func validateProfile(document gltf) error {
	if len(
		document.Animations,
	)+len(
		document.Skins,
	)+len(
		document.Cameras,
	)+len(
		document.ExtensionsUsed,
	)+len(
		document.ExtensionsRequired,
	) != 0 {
		return fmt.Errorf("animations, skins, cameras or extensions: %w", errUnsupported)
	}

	if len(document.Nodes) == 0 || len(document.Nodes) > maxObjects || len(document.Meshes) == 0 ||
		len(document.Meshes) > maxObjects || len(document.Materials) > maxObjects || len(document.Accessors) > maxObjects ||
		len(document.Images) > modeldata.MaxTextures || len(document.Textures) > modeldata.MaxTextures || len(document.Scenes) == 0 {
		return errBounds
	}

	return nil
}

func validateBuffers(document gltf, binaryData []byte) error {
	if len(document.Buffers) != 1 || document.Buffers[0].URI != "" {
		return fmt.Errorf("external or multiple buffers: %w", errUnsupported)
	}

	length := document.Buffers[0].ByteLength
	if length < 0 || length > len(binaryData) || len(binaryData)-length >= wordSize {
		return errFormat
	}

	for _, value := range binaryData[length:] {
		if value != 0 {
			return errFormat
		}
	}

	for index, view := range document.BufferViews {
		if view.Buffer != 0 || view.ByteOffset < 0 || view.ByteLength < 1 || view.ByteOffset > length ||
			view.ByteLength > length-view.ByteOffset ||
			view.ByteStride != 0 && (view.ByteStride < wordSize || view.ByteStride > 252 || view.ByteStride%wordSize != 0) {
			return fmt.Errorf("bufferView %d: %w", index, errFormat)
		}
	}

	return nil
}

func validateAccessor(value accessor, views int) error {
	if value.BufferView == nil || *value.BufferView < 0 || *value.BufferView >= views || value.Count < 1 ||
		value.Count > meshdata.MaxIndices || value.ByteOffset < 0 || value.Normalized || len(value.Sparse) != 0 {
		return fmt.Errorf("sparse/normalized/missing or invalid accessor: %w", errUnsupported)
	}

	return nil
}

func validateRoots(document gltf, parents []int) error {
	for _, scene := range document.Scenes {
		seen := make(map[int]bool)
		for _, root := range scene.Nodes {
			if root < 0 || root >= len(document.Nodes) || parents[root] != 0 || seen[root] {
				return fmt.Errorf("scene roots: %w", errFormat)
			}

			seen[root] = true
		}
	}

	return nil
}
