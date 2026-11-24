//go:build !js

package level

import (
	"context"
	"errors"
	"io/fs"
	"path/filepath"
	"slices"
	"testing"

	"github.com/karty-game/ashura/internal/leveldata"
	"github.com/karty-game/ashura/pkg/texture"
)

func readChunkFixture(
	ctx context.Context,
	base string,
	chunk Chunk,
	resident map[string]imageMetadata,
	format texture.Format,
) (*PreparedChunk, error) {
	request, err := newReadRequest(chunk, format, fixtureTextureSupport(format), nil)
	if err != nil {
		return nil, err
	}

	request.resident = resident

	return request.Read(ctx, base)
}

func fixtureTextureSupport(format texture.Format) func(texture.Format) bool {
	return func(required texture.Format) bool {
		if format == texture.BC1 {
			return required == texture.BC1 || required == texture.BC6H || required == texture.BC7
		}

		return format == texture.ETC2RGB && (required == texture.ETC2RGB || required == texture.ETC2RGBA)
	}
}

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

			prepared, err := readChunkFixture(t.Context(), missing, chunk, nil, test.format)
			if prepared != nil || !errors.Is(err, ErrNativeHDR) {
				t.Fatalf("unsupported device reached payload I/O: prepared=%v, error=%v", prepared, err)
			}
		})
	}

	if err := validateDeviceProfile(chunk, texture.BC1, fixtureTextureSupport(texture.BC1), false); err != nil {
		t.Fatal("BC-capable device rejected native HDR", err)
	}

	prepared, err := readChunkFixture(t.Context(), missing, chunk, nil, texture.BC1)
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

			prepared, err := readChunkFixture(t.Context(), root, chunk, nil, test.format)
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

	if err := validateDeviceProfile(chunk, texture.BC1, fixtureTextureSupport(texture.BC1), false); err != nil {
		t.Fatal("fixture rejected before role admission", err)
	}

	prepared, err := readChunkFixture(t.Context(), root, chunk, nil, texture.BC1)
	if prepared != nil || !errors.Is(err, leveldata.ErrFormat) {
		t.Fatalf("raw RNM admitted in native HDR role: prepared=%v, error=%v", prepared, err)
	}
}

func TestMobileHDRChecksSSBumpSupportBeforePayloadIO(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	chunk := fixtureChunk(t, root)
	chunk.Pages[0].Storage, chunk.Pages[0].Encoding = mobileStorage, "rgbe8"
	missing := filepath.Join(root, "missing-payload-root")

	for _, test := range []struct {
		formats []texture.Format
		want    error
	}{
		{[]texture.Format{texture.ETC2RGB, texture.ETC2RGBA}, ErrNativeTextures},
		{[]texture.Format{texture.ASTC4x4}, ErrNativeTextures},
		{[]texture.Format{texture.ETC2RGB, texture.ETC2RGBA, texture.ASTC4x4}, fs.ErrNotExist},
	} {
		supports := func(format texture.Format) bool { return slices.Contains(test.formats, format) }

		request, err := newReadRequest(chunk, texture.ETC2RGB, supports, nil)
		if err == nil {
			_, err = request.Read(t.Context(), missing)
		}

		if !errors.Is(err, test.want) {
			t.Fatalf("formats %v returned %v, want %v", test.formats, err, test.want)
		}
	}

	for index := range chunk.Pages[0].Materials {
		chunk.Pages[0].Materials[index].SSBump = nil
	}

	if _, err := newReadRequest(chunk, texture.ETC2RGB, fixtureTextureSupport(texture.ETC2RGB), nil); err != nil {
		t.Fatal("ETC2-only device rejected an HDR page without SSBump textures", err)
	}
}
