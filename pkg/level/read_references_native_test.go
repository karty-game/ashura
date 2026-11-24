//go:build !js

package level

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/karty-game/ashura/pkg/scene/resource"
	"github.com/karty-game/ashura/pkg/texture"
)

func TestParallelChunkReadKeepsContentDeduplication(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	chunk := fixtureChunk(t, root)
	// The second lightmap names the same content through an absent alias. Stable
	// content deduplication must fetch the first reference exactly once.
	chunk.Pages[0].Maps[1].Path = "absent-lightmap-alias.ktx2"

	prepared, err := ReadChunk(t.Context(), root, chunk)
	if err != nil {
		t.Fatal("content alias triggered another read", err)
	}

	if len(prepared.payloads) != 5 || len(prepared.images) != 3 {
		t.Fatal("parallel reads duplicated shared content", len(prepared.payloads), len(prepared.images))
	}
}

func TestParallelChunkReadRejectsChecksumWithoutPartialResult(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	chunk := fixtureChunk(t, root)
	ref := chunk.Pages[0].Maps[0]
	//nolint:gosec // The reference is a checksummed fixture path beneath this test's private root.
	data, err := os.ReadFile(filepath.Join(root, ref.Path))
	if err != nil {
		t.Fatal(err)
	}

	data[0] ^= 255
	//nolint:gosec // Corrupt only the same checksummed fixture file in the test's private root.
	if err = os.WriteFile(filepath.Join(root, ref.Path), data, 0o600); err != nil {
		t.Fatal(err)
	}

	prepared, err := ReadChunk(t.Context(), root, chunk)
	if !errors.Is(err, resource.ErrInvalid) || prepared != nil {
		t.Fatal("parallel read masked integrity failure or returned partial resources", prepared, err)
	}
}

func TestParallelChunkReadPreflightsResidentSizesBeforeIO(t *testing.T) {
	t.Parallel()
	root := t.TempDir()

	chunk := fixtureChunk(t, root)
	if err := os.Remove(filepath.Join(root, chunk.Pages[0].Geometry.Path)); err != nil {
		t.Fatal(err)
	}

	ref := chunk.Pages[0].Maps[0]
	resident := map[string]imageMetadata{ref.SHA256: {size: ref.Size + 1}}

	prepared, err := readChunkFixture(t.Context(), root, chunk, resident, texture.RGBA8)
	if !errors.Is(err, ErrFormat) || prepared != nil {
		t.Fatal("I/O started before complete resident-snapshot admission", prepared, err)
	}

	canceled, cancel := context.WithCancel(t.Context())
	cancel()

	if result, err := ReadChunk(canceled, root, chunk); !errors.Is(err, context.Canceled) || result != nil {
		t.Fatal("canceled chunk started file I/O", result, err)
	}
	//nolint:staticcheck // Negative admission test: the public API must reject a nil context.
	if result, err := ReadChunk(nil, root, chunk); !errors.Is(err, ErrFormat) || result != nil {
		t.Fatal("nil context was admitted", result, err)
	}
}
