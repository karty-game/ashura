//go:build !js

package modelbuild

import (
	"fmt"

	"github.com/karty-game/karty-webgpu/internal/meshdata"
	"github.com/karty-game/karty-webgpu/internal/modeldata"
)

// Result reports exact prepared geometry/texture sizes, without runtime work.
type Result struct {
	Data                                                   []byte
	Parts, Textures, VertexBytes, IndexBytes, TextureBytes int
}

// Options controls offline embedded-texture preparation. Zero keeps source dimensions.
type Options struct{ MaxTextureDimension int }

type compiler struct {
	options           Options
	document          gltf
	binary            []byte
	output            modeldata.Document
	materials         []modeldata.Material
	textureCache      map[textureKey]int32
	vertices, indices int
	textureBytes      int
}

// Compile parses and flattens the supported static GLB profile offline. All
// transforms, normals, geometry packing and texture mips are final GPU payloads.
func Compile(source []byte, identity string) (Result, error) {
	return CompileWithOptions(source, identity, Options{})
}

// CompileWithOptions prepares the same geometry with an optional source texture cap.
func CompileWithOptions(source []byte, identity string, options Options) (Result, error) {
	if options.MaxTextureDimension < 0 || options.MaxTextureDimension > 4096 {
		return Result{}, errBounds
	}

	document, binaryData, err := readGLB(source)
	if err != nil {
		return Result{}, fmt.Errorf("model %s: %w", identity, err)
	}

	if err := validateDocument(document, binaryData); err != nil {
		return Result{}, fmt.Errorf("model %s: %w", identity, err)
	}

	build := compiler{
		options:      options,
		document:     document,
		binary:       binaryData,
		output:       modeldata.Document{Source: identity},
		textureCache: make(map[textureKey]int32),
	}
	if err := build.prepareMaterials(); err != nil {
		return Result{}, fmt.Errorf("model %s: %w", identity, err)
	}

	selected := 0
	if document.Scene != nil {
		selected = *document.Scene
	}

	if selected < 0 || selected >= len(document.Scenes) {
		return Result{}, errFormat
	}

	for _, root := range document.Scenes[selected].Nodes {
		if err := build.flatten(root, identityMatrix(), 0); err != nil {
			return Result{}, fmt.Errorf("model %s: %w", identity, err)
		}
	}

	prepared, err := modeldata.Encode(build.output)
	if err != nil {
		return Result{}, fmt.Errorf("model %s: %w", identity, err)
	}

	result := Result{Data: prepared, Parts: len(build.output.Parts), Textures: len(build.output.Textures),
		VertexBytes: build.vertices * meshdata.VertexStride, IndexBytes: build.indices * meshdata.IndexStride}
	for _, image := range build.output.Textures {
		result.TextureBytes += len(image.Data)
	}

	return result, nil
}

func (build *compiler) flatten(index int, parent matrix, depth int) error {
	if depth >= maxJSONDepth {
		return errBounds
	}

	source := build.document.Nodes[index]

	local, err := localMatrix(source)
	if err != nil {
		return err
	}

	transform, err := checkedMatrix(multiply(parent, local))
	if err != nil {
		return err
	}

	if source.Mesh != nil {
		value := build.document.Meshes[*source.Mesh]
		if len(value.Weights) != 0 || len(value.Primitives) == 0 {
			return fmt.Errorf("mesh weights or empty primitives: %w", errUnsupported)
		}

		for partIndex, primitive := range value.Primitives {
			if len(build.output.Parts) >= modeldata.MaxParts {
				return errBounds
			}

			name := fmt.Sprintf("%s/node%d/%s/primitive%d", source.Name, index, value.Name, partIndex)

			part, err := build.preparePart(primitive, transform, name)
			if err != nil {
				return fmt.Errorf("%s: %w", name, err)
			}

			build.output.Parts = append(build.output.Parts, part)
		}
	}

	for _, child := range source.Children {
		if err := build.flatten(child, transform, depth+1); err != nil {
			return err
		}
	}

	return nil
}
