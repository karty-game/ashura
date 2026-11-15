//go:build !js

package level

import (
	"errors"
	"io/fs"
	"path/filepath"
	"testing"

	"github.com/karty-game/karty-webgpu/internal/leveldata"
	"github.com/karty-game/karty-webgpu/pkg/texture"
)

func TestNativeHDRDeviceProfileRejectsBeforePayloadIO(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	chunk := fixtureChunk(t, root)
	chunk.Pages[0].Storage = "bc-hdr"
	chunk.Pages[0].Encoding = "linear"
	missing := filepath.Join(root, "missing-payload-root")

	for _, test := range []struct {
		name   string
		format texture.Format
	}{
		{name: "raw", format: texture.RGBA8},
		{name: "ETC2", format: texture.ETC2RGB},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			prepared, err := readChunk(t.Context(), missing, chunk, nil, nil, test.format)
			if prepared != nil || !errors.Is(err, ErrNativeHDR) {
				t.Fatalf("unsupported device reached payload I/O: prepared=%v, error=%v", prepared, err)
			}
		})
	}

	if err := validateDeviceProfile(chunk, texture.BC1, false); err != nil {
		t.Fatal("BC-capable device rejected native HDR", err)
	}

	prepared, err := readChunk(t.Context(), missing, chunk, nil, nil, texture.BC1)
	if prepared != nil || !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("BC-capable device did not proceed to payload I/O: prepared=%v, error=%v", prepared, err)
	}
}

func TestRawChunkLoadsOnEveryDeviceProfile(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	chunk := fixtureChunk(t, root)

	for _, test := range []struct {
		name   string
		format texture.Format
	}{
		{name: "raw", format: texture.RGBA8},
		{name: "BC", format: texture.BC1},
		{name: "ETC2", format: texture.ETC2RGB},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			prepared, err := readChunk(t.Context(), root, chunk, nil, nil, test.format)
			if err != nil {
				t.Fatal("existing raw chunk rejected", err)
			}

			if len(prepared.pages) != len(chunk.Pages) {
				t.Fatal("existing raw chunk lost admitted pages")
			}
		})
	}
}

func TestNativeHDRRoleRejectsRawRNMOnBCDevice(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	chunk := fixtureChunk(t, root)
	chunk.Pages[0].Storage = "bc-hdr"
	chunk.Pages[0].Encoding = "linear"

	if err := validateDeviceProfile(chunk, texture.BC1, false); err != nil {
		t.Fatal("fixture rejected before role admission", err)
	}

	prepared, err := readChunk(t.Context(), root, chunk, nil, nil, texture.BC1)
	if prepared != nil || !errors.Is(err, leveldata.ErrFormat) {
		t.Fatalf("raw RNM admitted in native HDR role: prepared=%v, error=%v", prepared, err)
	}
}
