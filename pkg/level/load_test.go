//go:build !js

//nolint:gosec,varnamelen // Test files use owned temporary roots and checksummed names; uv names the secondary UV stream.
package level

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"testing"

	"github.com/karty-game/karty-webgpu/internal/leveldata"
	"github.com/karty-game/karty-webgpu/internal/modeldata"
	"github.com/karty-game/karty-webgpu/internal/texturedata"
	"github.com/karty-game/karty-webgpu/pkg/assetbuild"
	"github.com/karty-game/karty-webgpu/pkg/engine"
	"github.com/karty-game/karty-webgpu/pkg/material"
)

func payload(t *testing.T, root string, data []byte) Reference {
	t.Helper()

	path := fmt.Sprintf("%x.bin", sha256.Sum256(data))
	if err := os.WriteFile(filepath.Join(root, path), data, 0o600); err != nil {
		t.Fatal(err)
	}

	ref, err := leveldata.Reference(path, data)
	if err != nil {
		t.Fatal(err)
	}

	return ref
}
func fixtureTexture(t *testing.T, usage string, size int, pixel [4]byte) []byte {
	t.Helper()

	data, err := assetbuild.EncodeTexture(
		assetbuild.Image{
			Name:   "Fixture",
			Source: "fixture",
			Usage:  usage,
			Width:  size,
			Height: size,
			Levels: [][]byte{bytes.Repeat(pixel[:], size*size)},
		},
	)
	if err != nil {
		t.Fatal(err)
	}

	return data
}
func fixtureChunk(t *testing.T, root string) Chunk {
	t.Helper()

	part := modeldata.Part{
		Name:     "batch_0",
		Material: modeldata.Material{Color: [4]float32{1, 1, 1, 1}, Roughness: .5, Albedo: -1, Normal: -1, AO: -1},
	}

	for _, point := range [][3]float32{{0, 0, 0}, {0, 0, 1}, {1, 0, 0}} {
		for _, value := range []float32{point[0], point[1], point[2], 0, 1, 0, point[0], point[2]} {
			part.Vertices = binary.LittleEndian.AppendUint32(part.Vertices, math.Float32bits(value))
		}
	}

	for _, index := range []uint32{0, 1, 2} {
		part.Indices = binary.LittleEndian.AppendUint32(part.Indices, index)
	}

	geometry, err := modeldata.Encode(modeldata.Document{Source: "fixture", Parts: []modeldata.Part{part}})
	if err != nil {
		t.Fatal(err)
	}

	uv, err := leveldata.EncodeUV(sha256.Sum256(geometry), [][]byte{make([]byte, 24)})
	if err != nil {
		t.Fatal(err)
	}

	albedo := payload(t, root, fixtureTexture(t, assetbuild.UsageAlbedo, 4, [4]byte{255, 255, 255, 255}))
	bump := payload(t, root, fixtureTexture(t, assetbuild.UsageSSBump, 4, [4]byte{85, 85, 85, 255}))
	lightmap := payload(t, root, fixtureTexture(t, assetbuild.UsageLightmap, 16, [4]byte{128, 64, 32, 128}))
	page := Page{
		Geometry:  payload(t, root, geometry),
		UV:        payload(t, root, uv),
		Maps:      [3]Reference{lightmap, lightmap, lightmap},
		Width:     16,
		Height:    16,
		Scale:     2,
		Encoding:  "rgbe8",
		Materials: []Material{{Name: "batch_0", Color: [4]float32{1, 1, 1, 1}, Albedo: &albedo, SSBump: &bump}},
	}

	return Chunk{ID: "default/0/0/0", Space: "default", Max: [3]float32{32, 32, 32}, Pages: []Page{page}}
}

func TestChunksShareTexturesAndKeepIndependentPresentationLifetime(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	description := fixtureChunk(t, root)

	prepared, err := ReadChunk(t.Context(), root, description)
	if err != nil {
		t.Fatal(err)
	}
	// A caller editing its source descriptor cannot mutate an admitted chunk.
	description.Pages[0].Materials[0].Name = "mutated"
	description.Pages[0].Materials[0].Albedo.SHA256 = "mutated"

	app := engine.NewApp(engine.AppConfig{})
	defer func() { _ = app.Close() }()

	loader := NewLoader(app)

	first, err := loader.Load(prepared)
	if err != nil {
		t.Fatal(err)
	}

	second, err := loader.Load(prepared)
	if err != nil {
		t.Fatal(err)
	}

	if len(loader.textures) != 3 {
		t.Fatalf("shared RNM/albedo/SSBump textures duplicated: %d", len(loader.textures))
	}

	for _, image := range loader.textures {
		if image.references != 2 {
			t.Fatal("references counted per material/map rather than owning chunk")
		}
	}

	world := engine.NewWorld()

	one, err := first.Instantiate(world)
	if err != nil {
		t.Fatal(err)
	}

	_, err = second.Instantiate(world)
	if err != nil || world.Len() != 2 {
		t.Fatalf("independent instances: %v", err)
	}

	world.Each(func(entity engine.Entity) {
		source, ok := engine.Get[material.SourceMaterial](world, entity)
		if !ok || source.RNMLightmap == nil || source.RNMLightmap.Encoding != texturedata.RNMRGBE || source.SSBumpStrength != 1 {
			t.Fatal("chunk did not use complete Source RNM material")
		}
	})

	if err = first.Unload(); !errors.Is(err, ErrInUse) || !errors.Is(err, engine.ErrInUse) || errors.Is(err, ErrClosed) {
		t.Fatalf("live presentation must return in-use, not closed: %v", err)
	}

	if err = loader.Close(); !errors.Is(err, ErrInUse) {
		t.Fatalf("live loader must return in-use: %v", err)
	}

	one.Remove()

	if err = first.Unload(); err != nil {
		t.Fatal(err)
	}

	for _, image := range loader.textures {
		if image.references != 1 || !texturedata.Valid(image.image) {
			t.Fatal("unloading a chunk released a shared texture")
		}
	}

	if err = app.Close(); err != nil {
		t.Fatal(err)
	}

	if !loader.closed || !second.closed || len(loader.textures) != 0 || len(loader.chunks) != 0 || world.Len() != 0 {
		t.Fatal("App close retained chunk ownership/presentation")
	}

	if _, err = second.Instantiate(world); !errors.Is(err, ErrClosed) || errors.Is(err, ErrInUse) {
		t.Fatalf("closed chunk must return closed, not in-use: %v", err)
	}
}

func TestProductionChunkLoadDoesNotRequestDeveloperBakeInputs(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	description := fixtureChunk(t, root)

	developer, err := leveldata.Reference("missing-developer-input.klightmap", []byte{1})
	if err != nil {
		t.Fatal(err)
	}

	description.Pages[0].BakeInputs = &developer

	prepared, err := ReadChunk(t.Context(), root, description)
	if err != nil {
		t.Fatal(err)
	}

	developer.Path = "changed-by-caller"
	if prepared.chunk.Pages[0].BakeInputs.Path != "missing-developer-input.klightmap" {
		t.Fatal("prepared developer descriptor aliases caller metadata")
	}

	app := engine.NewApp(engine.AppConfig{})
	defer func() { _ = app.Close() }()

	loader := NewLoader(app)

	chunk, err := loader.Load(prepared)
	if err != nil {
		t.Fatal(err)
	}

	if err = chunk.Unload(); err != nil {
		t.Fatal(err)
	}
}

func TestChunkReadRejectsMismatchedFamiliesBeforeResourceCreation(t *testing.T) {
	t.Parallel()
	root := t.TempDir()

	original := fixtureChunk(t, root)
	for name, change := range map[string]func(*Chunk){
		"UV binding": func(chunk *Chunk) {
			uv, err := leveldata.EncodeUV(sha256.Sum256([]byte("different geometry")), [][]byte{make([]byte, 24)})
			if err != nil {
				t.Fatal(err)
			}

			chunk.Pages[0].UV = payload(t, root, uv)
		},
		"material roster": func(chunk *Chunk) { chunk.Pages[0].Materials[0].Name = "wrong" },
		"map encoding":    func(chunk *Chunk) { chunk.Pages[0].Maps[0] = *chunk.Pages[0].Materials[0].Albedo },
		"map dimensions": func(chunk *Chunk) {
			chunk.Pages[0].Maps[0] = payload(t, root, fixtureTexture(t, assetbuild.UsageLightmap, 32, [4]byte{1, 1, 1, 1}))
		},
		"corrupt map": func(chunk *Chunk) { chunk.Pages[0].Maps[0] = payload(t, root, []byte{1, 2, 3}) },
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			chunk := cloneChunk(original)
			change(&chunk)

			if _, err := ReadChunk(t.Context(), root, chunk); err == nil {
				t.Fatal("inconsistent family admitted")
			}
		})
	}
}

func TestChunkRejectsMaskedGeometryWithValidBinding(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	chunk := fixtureChunk(t, root)

	if _, err := ReadChunk(t.Context(), root, chunk); err != nil {
		t.Fatal("opaque control chunk failed admission", err)
	}

	geometry, err := os.ReadFile(filepath.Join(root, chunk.Pages[0].Geometry.Path))
	if err != nil {
		t.Fatal(err)
	}

	document, err := modeldata.Decode(geometry)
	if err != nil {
		t.Fatal(err)
	}

	document.Parts[0].Material.AlphaMask = true
	document.Parts[0].Material.AlphaCutoff = .375

	geometry, err = modeldata.Encode(document)
	if err != nil {
		t.Fatal(err)
	}

	uv, err := leveldata.EncodeUV(sha256.Sum256(geometry), [][]byte{make([]byte, 24)})
	if err != nil {
		t.Fatal(err)
	}

	chunk.Pages[0].Geometry = payload(t, root, geometry)
	chunk.Pages[0].UV = payload(t, root, uv)

	if _, err = ReadChunk(t.Context(), root, chunk); !errors.Is(err, ErrFormat) {
		t.Fatal("static geometry admitted alpha coverage absent from its level material contract", err)
	}
}

func TestFailedLoadRollsBackSharedResourceReferences(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	description := fixtureChunk(t, root)

	good, err := ReadChunk(t.Context(), root, description)
	if err != nil {
		t.Fatal(err)
	}

	app := engine.NewApp(engine.AppConfig{})
	defer func() { _ = app.Close() }()

	loader := NewLoader(app)

	resident, err := loader.Load(good)
	if err != nil {
		t.Fatal(err)
	}

	geometry, err := os.ReadFile(filepath.Join(root, description.Pages[0].Geometry.Path))
	if err != nil {
		t.Fatal(err)
	}

	document, err := modeldata.Decode(geometry)
	if err != nil {
		t.Fatal(err)
	}

	document.Source = "second-page"

	geometry, err = modeldata.Encode(document)
	if err != nil {
		t.Fatal(err)
	}

	uv, err := leveldata.EncodeUV(sha256.Sum256(geometry), [][]byte{make([]byte, 24)})
	if err != nil {
		t.Fatal(err)
	}

	second := description.Pages[0]
	second.Geometry = payload(t, root, geometry)
	second.UV = payload(t, root, uv)
	description.Pages = append(description.Pages, second)

	broken, err := ReadChunk(t.Context(), root, description)
	if err != nil {
		t.Fatal(err)
	}
	// Inject a later-page failure to exercise rollback after sharing first-page textures.
	changed, err := modeldata.Decode(broken.pages[1].geometry)
	if err != nil {
		t.Fatal(err)
	}

	binary.LittleEndian.PutUint32(changed.Parts[0].Vertices, math.Float32bits(float32(math.NaN())))

	if loaded, err := loader.Load(broken); err == nil || loaded != nil {
		t.Fatal("failed generation was published")
	}

	if len(loader.chunks) != 1 || !loader.chunks[resident] {
		t.Fatal("rollback removed an existing resident chunk")
	}

	for _, image := range loader.textures {
		if image.references != 1 || !texturedata.Valid(image.image) {
			t.Fatal("rollback corrupted shared texture lifetime")
		}
	}

	if err = resident.Unload(); err != nil {
		t.Fatal(err)
	}

	if len(loader.textures) != 0 {
		t.Fatal("rollback left leaked texture references")
	}
}

func TestRejectedTextureReleaseLeavesCountsAndGeometryIntact(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	description := fixtureChunk(t, root)

	prepared, err := ReadChunk(t.Context(), root, description)
	if err != nil {
		t.Fatal(err)
	}

	app := engine.NewApp(engine.AppConfig{})
	defer func() { _ = app.Close() }()

	loader := NewLoader(app)

	chunk, err := loader.Load(prepared)
	if err != nil {
		t.Fatal(err)
	}

	key := description.Pages[0].Materials[0].Albedo.SHA256

	shared := loader.textures[key]
	if err = app.UnloadTexture(shared.image); err != nil {
		t.Fatal(err)
	}

	if err = chunk.Unload(); err == nil {
		t.Fatal("stale shared handle accepted for release")
	}

	if shared.references != 1 || chunk.closed || len(chunk.pages) != 1 || !loader.chunks[chunk] {
		t.Fatal("failed release partially changed chunk ownership")
	}

	shared.image, err = app.NewTexture(prepared.images[key])
	if err != nil {
		t.Fatal(err)
	}

	if err = chunk.Unload(); err != nil {
		t.Fatal(err)
	}
}
