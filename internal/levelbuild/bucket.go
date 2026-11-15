//go:build !js

//nolint:mnd,gosec // Fixed GPU ABI offsets; texture/vertex/index counters are bounded before narrowing.
package levelbuild

import (
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"math"

	"github.com/karty-game/karty-webgpu/internal/meshdata"
	"github.com/karty-game/karty-webgpu/internal/modeldata"
)

type batchKey struct {
	material        modeldata.Material
	group, override string
}

type batch struct {
	part     modeldata.Part
	vertices map[[8]uint32]uint32
}

type textureKey struct {
	role modeldata.TextureRole
	hash [32]byte
}

type textureIdentity struct {
	data *byte
	size int
}

type bucket struct {
	source            string
	textures          []modeldata.Texture
	textureIndices    map[textureKey]int32
	parts             []batch
	partIndices       map[batchKey]int
	sources           []SourceRange
	vertices, indices int
	bytes             uint64
}

func newBucket(kind, identity string) *bucket {
	source := fmt.Sprintf("%s:%x", kind, sha256.Sum256([]byte(identity)))

	return &bucket{source: source, bytes: uint64(28 + (len(source)+3)/4*4),
		textureIndices: make(map[textureKey]int32), partIndices: make(map[batchKey]int)}
}

func (output *bucket) material(
	placement Placement,
	original modeldata.Part,
	hashes map[textureIdentity][32]byte,
) (modeldata.Material, error) {
	material := original.Material

	slots := []*int32{&material.Albedo, &material.Normal, &material.AO}
	for _, slot := range slots {
		if *slot < 0 {
			continue
		}

		image := placement.Model.Textures[*slot]
		identity := textureIdentity{data: &image.Data[0], size: len(image.Data)}

		hash, found := hashes[identity]
		if !found {
			hash = sha256.Sum256(image.Data)
			hashes[identity] = hash
		}

		key := textureKey{role: image.Role, hash: hash}

		index, found := output.textureIndices[key]
		if !found {
			if len(output.textures) >= modeldata.MaxTextures {
				return modeldata.Material{}, ErrGeometry
			}

			output.bytes += uint64(8 + (len(image.Data)+3)/4*4)
			if output.bytes > modeldata.MaxBytes {
				return modeldata.Material{}, ErrGeometry
			}

			index = int32(len(output.textures))
			output.textureIndices[key] = index
			output.textures = append(output.textures, image)
		}

		*slot = index
	}

	return material, nil
}

func (output *bucket) add(value triangle, placement Placement, original modeldata.Part,
	hashes map[textureIdentity][32]byte,
) error {
	material, err := output.material(placement, original, hashes)
	if err != nil {
		return err
	}

	key := batchKey{material: material, group: placement.Group, override: placement.PartGroups[original.Name]}

	index, found := output.partIndices[key]
	if !found {
		if len(output.parts) >= modeldata.MaxParts {
			return ErrGeometry
		}

		index = len(output.parts)
		name := fmt.Sprintf("batch_%d", index)
		output.bytes += uint64(56 + (len(name)+3)/4*4)
		output.partIndices[key] = index
		output.parts = append(
			output.parts,
			batch{part: modeldata.Part{Name: name, Material: material}, vertices: make(map[[8]uint32]uint32)},
		)
	}

	part := &output.parts[index]
	start := uint32(len(part.part.Indices) / meshdata.IndexStride)

	for _, vertex := range value {
		words := encodeVertex(vertex)

		vertexIndex, exists := part.vertices[words]
		if !exists {
			output.vertices++

			output.bytes += meshdata.VertexStride
			if output.vertices > meshdata.MaxVertices || output.bytes > modeldata.MaxBytes {
				return ErrGeometry
			}

			vertexIndex = uint32(len(part.part.Vertices) / meshdata.VertexStride)

			part.vertices[words] = vertexIndex
			for _, word := range words {
				part.part.Vertices = binary.LittleEndian.AppendUint32(part.part.Vertices, word)
			}
		}

		output.indices++

		output.bytes += meshdata.IndexStride
		if output.indices > meshdata.MaxIndices || output.bytes > modeldata.MaxBytes {
			return ErrGeometry
		}

		part.part.Indices = binary.LittleEndian.AppendUint32(part.part.Indices, vertexIndex)
	}

	output.provenance(SourceRange{ID: placement.ID, Name: original.Name, Group: placement.Group,
		Part: index, FirstIndex: start, IndexCount: 3})

	return nil
}

func (output *bucket) provenance(value SourceRange) {
	if len(output.sources) > 0 {
		last := &output.sources[len(output.sources)-1]
		if last.ID == value.ID && last.Name == value.Name && last.Group == value.Group && last.Part == value.Part &&
			last.FirstIndex+last.IndexCount == value.FirstIndex {
			last.IndexCount += value.IndexCount

			return
		}
	}

	output.sources = append(output.sources, value)
}

func (output *bucket) document() modeldata.Document {
	parts := make([]modeldata.Part, len(output.parts))
	for index := range output.parts {
		parts[index] = output.parts[index].part
	}

	return modeldata.Document{Source: output.source, Textures: output.textures, Parts: parts}
}

func encodeVertex(value vertex) [8]uint32 {
	value.normal = unit(value.normal)

	var words [8]uint32
	for axis := range 3 {
		words[axis] = math.Float32bits(float32(value.position[axis]))
		words[axis+3] = math.Float32bits(float32(value.normal[axis]))
	}

	words[6], words[7] = math.Float32bits(float32(value.uv[0])), math.Float32bits(float32(value.uv[1]))

	return words
}
