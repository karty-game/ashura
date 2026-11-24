//go:build !js

//nolint:gosec // Every payload path is a bounded fixture beneath the test's private temporary root.
package level

import (
	"archive/zip"
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/karty-game/ashura/internal/leveldata"
	"github.com/karty-game/ashura/internal/resourcedata"
	"github.com/karty-game/ashura/pkg/engine"
	"github.com/karty-game/ashura/pkg/scene/resource"
	"github.com/karty-game/ashura/pkg/texture"
)

func bundledChunkFixture(t *testing.T, root string) (Chunk, []byte) {
	t.Helper()
	chunk := fixtureChunk(t, root)

	owned, err := leveldata.ChunkOwnedReferences(chunk)
	if err != nil {
		t.Fatal(err)
	}

	var data bytes.Buffer

	writer := zip.NewWriter(&data)

	for _, ref := range owned {
		contents, readErr := os.ReadFile(filepath.Join(root, ref.Path))
		if readErr != nil {
			t.Fatal(readErr)
		}

		entry, createErr := writer.CreateHeader(&zip.FileHeader{Name: ref.SHA256, Method: zip.Store})
		if createErr != nil {
			t.Fatal(createErr)
		}

		if _, writeErr := entry.Write(contents); writeErr != nil {
			t.Fatal(writeErr)
		}
	}

	if err = writer.Close(); err != nil {
		t.Fatal(err)
	}

	archive := append([]byte(nil), data.Bytes()...)

	index, err := zip.NewReader(bytes.NewReader(archive), int64(len(archive)))
	if err != nil {
		t.Fatal(err)
	}

	chunk.Bundle = &leveldata.ChunkBundle{}

	for _, entry := range index.File {
		offset, entryErr := entry.DataOffset()
		if entryErr != nil {
			t.Fatal(entryErr)
		}

		chunk.Bundle.Entries = append(chunk.Bundle.Entries, leveldata.BundleEntry{SHA256: entry.Name, Offset: offset})
	}

	writeChunkBundleFixture(t, root, &chunk, archive)

	for _, ref := range owned {
		if err = os.Remove(filepath.Join(root, ref.Path)); err != nil {
			t.Fatal(err)
		}
	}

	return chunk, archive
}

func writeChunkBundleFixture(t *testing.T, root string, chunk *Chunk, data []byte) {
	t.Helper()

	ref, err := leveldata.Reference("owned-chunk.zip", data)
	if err != nil {
		t.Fatal(err)
	}

	if err = os.WriteFile(filepath.Join(root, ref.Path), data, 0600); err != nil {
		t.Fatal(err)
	}

	chunk.Bundle.File = ref
}

func TestBundledChunkReadsOwnedFileAndKeepsSharedTextureResidency(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	chunk, archive := bundledChunkFixture(t, root)

	prepared, err := ReadChunk(t.Context(), root, chunk)
	if err != nil {
		t.Fatal("bundled chunk requested an absent loose file", err)
	}

	owned, err := leveldata.ChunkOwnedReferences(chunk)
	if err != nil {
		t.Fatal(err)
	}

	if len(prepared.payloads) != 5 {
		t.Fatal("bundling changed the admitted logical dependencies")
	}

	for index, ref := range owned {
		offset := chunk.Bundle.Entries[index].Offset

		expected := archive[offset : offset+int64(ref.Size)]
		if !bytes.Equal(prepared.payloadBytes(ref.SHA256), expected) || !resourcedata.Valid(prepared.payloads[ref.SHA256]) {
			t.Fatal("bundle did not retain verified original payload", ref.Path)
		}
	}

	app := engine.NewApp(engine.AppConfig{})
	defer func() { _ = app.Close() }()

	loader := NewLoader(app)

	first, err := loader.Load(prepared)
	if err != nil {
		t.Fatal(err)
	}

	request, err := loader.Request(chunk)
	if err != nil {
		t.Fatal(err)
	}
	// A Request owns its complete index snapshot, including entry offsets.
	chunk.Bundle.File.Path = "absent-mutated.zip"
	chunk.Bundle.Entries[0].Offset++

	for _, ref := range []*Reference{chunk.Pages[0].Materials[0].Albedo, chunk.Pages[0].Materials[0].SSBump} {
		if err = os.Remove(filepath.Join(root, ref.Path)); err != nil {
			t.Fatal(err)
		}
	}

	secondPrepared, err := request.Read(t.Context(), root)
	if err != nil {
		t.Fatal("resident material textures were read again or the bundle index was aliased", err)
	}

	if len(secondPrepared.payloads) != 2 || len(secondPrepared.images) != 0 {
		t.Fatal("resident images were verified or decoded again", len(secondPrepared.payloads), len(secondPrepared.images))
	}

	second, err := loader.Load(secondPrepared)
	if err != nil {
		t.Fatal(err)
	}

	if len(loader.textures) != 3 {
		t.Fatal("bundles duplicated shared textures", len(loader.textures))
	}

	for _, shared := range loader.textures {
		if shared.references != 2 {
			t.Fatal("shared texture ownership changed")
		}
	}

	if err = first.Unload(); err != nil {
		t.Fatal(err)
	}

	if len(loader.textures) != 3 {
		t.Fatal("first chunk unload evicted the second chunk's textures")
	}

	if err = second.Unload(); err != nil {
		t.Fatal(err)
	}

	if len(loader.textures) != 0 {
		t.Fatal("bundle-backed chunk leaked texture references")
	}
}

func TestBundledChunkRejectsOuterAndInnerIntegrityFailures(t *testing.T) {
	t.Parallel()

	for _, failure := range []string{"outer-checksum", "inner-checksum", "entry-offset", "missing-entry"} {
		t.Run(failure, func(t *testing.T) {
			t.Parallel()
			root := t.TempDir()
			chunk, data := bundledChunkFixture(t, root)
			expected := resource.ErrInvalid

			switch failure {
			case "outer-checksum", "inner-checksum":
				data[chunk.Bundle.Entries[0].Offset] ^= 1
				if failure == "inner-checksum" {
					writeChunkBundleFixture(t, root, &chunk, data)
				} else if err := os.WriteFile(filepath.Join(root, chunk.Bundle.File.Path), data, 0600); err != nil {
					t.Fatal(err)
				}
			case "entry-offset":
				chunk.Bundle.Entries[0].Offset++
			case "missing-entry":
				chunk.Bundle.Entries = chunk.Bundle.Entries[:len(chunk.Bundle.Entries)-1]
				expected = ErrFormat
			}

			prepared, err := ReadChunk(t.Context(), root, chunk)
			if !errors.Is(err, expected) || prepared != nil {
				t.Fatal("bad bundle admitted or fell back to loose files", prepared, err)
			}
		})
	}
}

func TestBundledChunkPreflightsResidentSizesBeforeIO(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	chunk, _ := bundledChunkFixture(t, root)
	chunk.Bundle.File.Path = "missing-archive.zip"
	ref := chunk.Pages[0].Maps[0]
	resident := map[string]imageMetadata{ref.SHA256: {size: ref.Size + 1}}

	prepared, err := readChunkFixture(t.Context(), root, chunk, resident, texture.RGBA8)
	if !errors.Is(err, ErrFormat) || prepared != nil {
		t.Fatal("bundle I/O started before resident-snapshot admission", prepared, err)
	}
}
