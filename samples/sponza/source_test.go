package main

import (
	"encoding/binary"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/karty-game/ashura/internal/ktx2data"
	"github.com/karty-game/ashura/internal/meshdata"
	"github.com/karty-game/ashura/internal/modeldata"
)

const triangleCorners = 3

func TestSponzaSingleSourcePreservesGeometryAndMaskedCoverage(t *testing.T) {
	t.Parallel()

	const source = "assets/sponza.glb"

	data, err := os.ReadFile(source)
	if err != nil {
		t.Fatal(err)
	}

	// Check source coverage before chunk clipping and two-sided page expansion.
	// The standard CLI keeps host preparation imports out of the sample package.
	output := filepath.Join(t.TempDir(), "source.kmodel")
	//nolint:gosec // G204: fixed Go asset CLI; paths are a constant input and this test's private output.
	command := exec.CommandContext(t.Context(), "go", "run", "../../cmd/engine", "assets", "model", "--input", source, "--output", output)
	if diagnostics, err := command.CombinedOutput(); err != nil {
		t.Fatalf("prepare model through the asset CLI: %v\n%s", err, diagnostics)
	}

	//nolint:gosec // G304: the CLI output is one fixed filename in this test's private directory.
	prepared, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}

	document, err := modeldata.Decode(prepared)
	if err != nil {
		t.Fatal(err)
	}

	var opaque, masked, triangles int
	for _, part := range document.Parts {
		triangles += len(part.Indices) / (triangleCorners * meshdata.IndexStride)
		if !part.Material.AlphaMask {
			opaque++

			continue
		}

		masked++

		assertSourceCoverage(t, document, part)
	}

	if document.Source != source || opaque == 0 || masked == 0 || triangles != sourceTriangles(t, data) {
		t.Fatal("single source lost architecture, cutouts or original geometry", document.Source, opaque, masked, triangles)
	}
}

func assertSourceCoverage(t *testing.T, document modeldata.Document, part modeldata.Part) {
	t.Helper()

	material := part.Material
	if material.Albedo < 0 || material.AlphaCutoff < 0 || material.AlphaCutoff > 1 {
		t.Fatal("source cutout lost its material coverage", part.Name)
	}

	image, err := ktx2data.Decode(document.Textures[material.Albedo].Data)
	if err != nil || len(image.Levels) < 2 {
		t.Fatal("source cutout lost prepared alpha mips", part.Name, err)
	}

	const (
		channels    = 4
		opaqueAlpha = 255
	)

	var holes, solid bool

	for offset := channels - 1; offset < len(image.Levels[0]); offset += channels {
		coverage := float32(image.Levels[0][offset]) / opaqueAlpha * material.Color[3]
		holes = holes || coverage < material.AlphaCutoff
		solid = solid || coverage >= material.AlphaCutoff
	}

	if !holes || !solid {
		t.Fatal("source cutout no longer contains both holes and covered material", part.Name)
	}
}

type geometrySource struct {
	Scene  *int `json:"scene"`
	Scenes []struct {
		Nodes []int `json:"nodes"`
	} `json:"scenes"`
	Nodes []struct {
		Mesh     *int  `json:"mesh"`
		Children []int `json:"children"`
	} `json:"nodes"`
	Meshes []struct {
		Primitives []struct {
			Indices    *int           `json:"indices"`
			Attributes map[string]int `json:"attributes"`
		} `json:"primitives"`
	} `json:"meshes"`
	Accessors []struct {
		Count int `json:"count"`
	} `json:"accessors"`
}

// Count selected source triangles independently of compiler flattening and wire
// encoding. Material splitting must not drop or duplicate source geometry.
func sourceTriangles(t *testing.T, data []byte) int {
	t.Helper()

	const (
		jsonStart        = 20
		jsonLengthOffset = 12
	)

	if len(data) < jsonStart {
		t.Fatal("source is not a complete GLB")
	}

	length := uint64(binary.LittleEndian.Uint32(data[jsonLengthOffset:]))
	if length+jsonStart > uint64(len(data)) {
		t.Fatal("source JSON exceeds its GLB")
	}

	var source geometrySource
	if err := json.Unmarshal(data[jsonStart:jsonStart+length], &source); err != nil {
		t.Fatal(err)
	}

	scene := 0
	if source.Scene != nil {
		scene = *source.Scene
	}

	var visit func(int) int

	visit = func(index int) int {
		node := source.Nodes[index]
		count := 0

		if node.Mesh != nil {
			for _, primitive := range source.Meshes[*node.Mesh].Primitives {
				accessor := primitive.Attributes["POSITION"]
				if primitive.Indices != nil {
					accessor = *primitive.Indices
				}

				count += source.Accessors[accessor].Count / triangleCorners
			}
		}

		for _, child := range node.Children {
			count += visit(child)
		}

		return count
	}

	count := 0
	for _, root := range source.Scenes[scene].Nodes {
		count += visit(root)
	}

	return count
}
