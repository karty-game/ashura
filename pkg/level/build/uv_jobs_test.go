//go:build !js

package levelbuild

import (
	"bytes"
	"encoding/binary"
	"testing"

	"github.com/karty-game/karty-webgpu/internal/modeldata"
)

func TestReceiverJobsPreserveEveryTriangleAndMaterial(t *testing.T) {
	t.Parallel()
	project := writeFixture(t)

	data := readLevelFixture(t, project, "floor.kmodel")

	model, err := modeldata.Decode(data)
	if err != nil {
		t.Fatal(err)
	}

	jobs := receiverJobs(model, 1)
	if len(jobs) != 2 {
		t.Fatalf("wanted two bounded jobs, got %d", len(jobs))
	}

	original := make([]byte, 0, len(model.Parts[0].Indices)*8)

	actual := make([]byte, 0, cap(original))

	for _, part := range model.Parts {
		original = append(original, indexedVertices(part)...)
	}

	for _, job := range jobs {
		if len(job.Parts) != 1 || len(job.Parts[0].Indices) != 12 ||
			job.Parts[0].Name != model.Parts[0].Name || job.Parts[0].Material != model.Parts[0].Material ||
			len(job.Textures) != len(model.Textures) || !bytes.Equal(job.Textures[0].Data, model.Textures[0].Data) {
			t.Fatal("receiver job changed triangle bounds or material identity")
		}

		actual = append(actual, indexedVertices(job.Parts[0])...)
	}

	if !bytes.Equal(original, actual) {
		t.Fatal("receiver work split changed positions, normals, UV0, winding or triangle order")
	}
}

func indexedVertices(part modeldata.Part) []byte {
	var output []byte

	for offset := 0; offset < len(part.Indices); offset += 4 {
		index := int(binary.LittleEndian.Uint32(part.Indices[offset:]))
		output = append(output, part.Vertices[index*32:(index+1)*32]...)
	}

	return output
}
