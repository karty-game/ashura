//go:build !js

package levelbuild

import (
	"archive/zip"
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/karty-game/ashura/internal/leveldata"
	"github.com/karty-game/ashura/pkg/assetbuild"
)

func TestChunkBundlesPreserveOwnedBytesAndSharedMaterials(t *testing.T) {
	t.Parallel()
	project := writeFixture(t)
	config := fixtureConfig(fixtureChunkBundleBake())

	options := assetbuild.Options{Project: project, Identity: "chunk-bundle-publication"}
	if _, err := assetbuild.Build(t.Context(), config, options); err != nil {
		t.Fatal(err)
	}

	loose := findManifest(t, project)

	expected := chunkBundleFixtureBytes(t, project, loose)
	if len(loose.Chunks) < 2 {
		t.Fatal("fixture does not exercise shared textures across chunks")
	}

	config.Pipelines[0].Output = []assetbuild.Plugin{Publish(PublishOptions{BundleChunks: true})}

	cold, err := assetbuild.Build(t.Context(), config, options)
	if err != nil {
		t.Fatal(err)
	}

	bundled := findManifest(t, project)
	if bundled.Schema != leveldata.Schema8 || len(bundled.Chunks) != len(loose.Chunks) {
		t.Fatal("bundle publication changed chunk ownership")
	}

	if cold.Removed == 0 {
		t.Fatal("loose chunk payloads were not pruned")
	}

	assertChunkBundles(t, project, bundled, expected)
	assertLooseChunkFilesAbsent(t, project, bundled)
	assertSharedMaterialFiles(t, project, bundled)

	for _, object := range bundled.Objects {
		if object.Asset != nil {
			_ = readLevelFixture(t, project, "generated/public/"+object.Asset.Path)
		}
	}

	warm, err := assetbuild.Build(t.Context(), config, options)
	if err != nil || warm.Executed != 0 || warm.Reused != len(cold.Actions) {
		t.Fatal("bundle action did not reuse complete cache", warm, err)
	}

	archivePath := filepath.Join(project, "generated/public", bundled.Chunks[0].Bundle.File.Path)
	if err = os.Remove(archivePath); err != nil {
		t.Fatal(err)
	}

	repaired, err := assetbuild.Build(t.Context(), config, options)
	if err != nil || repaired.Executed != 0 || repaired.Written != 1 {
		t.Fatal("missing bundle was not repaired from cache", repaired, err)
	}

	assertChunkBundles(t, project, findManifest(t, project), expected)

	options.NoCache = true
	if rebuilt, err := assetbuild.Build(t.Context(), config, options); err != nil || rebuilt.Written != 0 {
		t.Fatal("uncached bundle generation changed deterministic output", rebuilt, err)
	}

	options.NoCache = false

	before := materialOutputFiles(t, filepath.Join(project, "generated/public"))
	if err = os.WriteFile(filepath.Join(project, "test.scene.yaml"), []byte("version: 99\n"), 0600); err != nil {
		t.Fatal(err)
	}

	if _, err = assetbuild.Build(t.Context(), config, options); err == nil {
		t.Fatal("invalid source accepted")
	}

	if after := materialOutputFiles(t, filepath.Join(project, "generated/public")); !reflect.DeepEqual(before, after) {
		t.Fatal("failed generation changed bundles or publication")
	}
}

func chunkBundleFixtureBytes(t *testing.T, project string, document leveldata.Document) map[string][]byte {
	t.Helper()

	expected := map[string][]byte{}

	for _, chunk := range document.Chunks {
		refs, err := leveldata.ChunkOwnedReferences(chunk)
		if err != nil {
			t.Fatal(err)
		}

		for _, ref := range refs {
			expected[ref.SHA256] = readLevelFixture(t, project, "generated/public/"+ref.Path)
		}
	}

	return expected
}

func assertChunkBundles(t *testing.T, project string, document leveldata.Document, expected map[string][]byte) {
	t.Helper()

	for _, chunk := range document.Chunks {
		if chunk.Bundle == nil {
			t.Fatal("chunk has no bundle")
		}

		data := readLevelFixture(t, project, "generated/public/"+chunk.Bundle.File.Path)

		archive, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
		if err != nil {
			t.Fatal(err)
		}

		refs, err := leveldata.ChunkOwnedReferences(chunk)
		if err != nil || len(archive.File) != len(refs) || len(chunk.Bundle.Entries) != len(refs) {
			t.Fatal("bundle does not contain exactly the chunk-owned files", err)
		}

		wantedBytes := storedZIPEndBytes + len(refs)*storedZIPEntryOverhead
		for _, ref := range refs {
			wantedBytes += ref.Size
		}

		if len(data) != wantedBytes {
			t.Fatal("ZIP capacity calculation disagrees with encoded metadata", len(data), wantedBytes)
		}

		for index, file := range archive.File {
			assertChunkBundleEntry(t, data, file, chunk.Bundle.Entries[index], expected)
		}
	}
}

func assertChunkBundleEntry(t *testing.T, data []byte, file *zip.File, entry leveldata.BundleEntry, expected map[string][]byte) {
	t.Helper()

	original, found := expected[file.Name]
	if !found || file.Method != zip.Store {
		t.Fatal("bundle contains a compressed or non-chunk asset", file.Name)
	}

	offset, err := file.DataOffset()
	if err != nil || entry.SHA256 != file.Name || entry.Offset != offset {
		t.Fatal("manifest entry disagrees with standard ZIP index", entry, err)
	}

	if !bytes.Equal(data[offset:offset+int64(len(original))], original) {
		t.Fatal("bundle changed payload bytes")
	}

	reader, err := file.Open()
	if err != nil {
		t.Fatal(err)
	}

	unpacked, err := io.ReadAll(reader)
	if err = errors.Join(err, reader.Close()); err != nil || !bytes.Equal(unpacked, original) {
		t.Fatal("ZIP entry failed its integrity check", err)
	}
}

func assertLooseChunkFilesAbsent(t *testing.T, project string, document leveldata.Document) {
	t.Helper()

	for _, chunk := range document.Chunks {
		refs, err := leveldata.ChunkOwnedReferences(chunk)
		if err != nil {
			t.Fatal(err)
		}

		for _, ref := range refs {
			if _, err := os.Stat(filepath.Join(project, "generated/public", ref.Path)); !errors.Is(err, os.ErrNotExist) {
				t.Fatal("bundled payload still published separately", ref.Path, err)
			}
		}
	}
}

func TestMixedChunkBundleVariantsKeepSharedLooseDependencies(t *testing.T) {
	t.Parallel()

	for _, bundledFirst := range []bool{false, true} {
		project := writeFixture(t)

		profiles := []PublishOptions{{Variant: "Loose"}, {Variant: "Bundled", BundleChunks: true}}
		if bundledFirst {
			profiles[0], profiles[1] = profiles[1], profiles[0]
		}

		config := fixtureConfig(fixtureChunkBundleBake())

		config.Pipelines[0].Output = []assetbuild.Plugin{PublishVariants(profiles...)}
		if _, err := assetbuild.Build(
			t.Context(),
			config,
			assetbuild.Options{Project: project, Identity: "mixed-bundle-variants"},
		); err != nil {
			t.Fatal(err)
		}

		loose := bundleVariantManifest(t, project, "Loose")

		bundled := bundleVariantManifest(t, project, "Bundled")
		if loose.Schema == leveldata.Schema8 || bundled.Schema != leveldata.Schema8 {
			t.Fatal("variant bundle options contaminated another profile")
		}

		for _, chunk := range loose.Chunks {
			if chunk.Bundle != nil {
				t.Fatal("loose profile acquired bundled delivery")
			}
		}

		expected := chunkBundleFixtureBytes(t, project, loose)
		assertChunkBundles(t, project, bundled, expected)
		assertSharedMaterialFiles(t, project, bundled)
	}
}

func bundleVariantManifest(t *testing.T, project, variant string) leveldata.Document {
	t.Helper()

	files, err := filepath.Glob(filepath.Join(project, "generated/public/levels/profiles", variant, "*.json"))
	if err != nil || len(files) != 1 {
		t.Fatal("missing profile manifest", files, err)
	}

	data, err := os.ReadFile(files[0])
	if err != nil {
		t.Fatal(err)
	}

	document, err := leveldata.Decode(data)
	if err != nil {
		t.Fatal(err)
	}

	return document
}

func assertSharedMaterialFiles(t *testing.T, project string, document leveldata.Document) {
	t.Helper()

	uses := map[string]int{}

	for _, chunk := range document.Chunks {
		for _, page := range chunk.Pages {
			for _, material := range page.Materials {
				for _, ref := range []*leveldata.Ref{material.Albedo, material.SSBump} {
					if ref == nil {
						continue
					}

					uses[ref.Path]++

					_ = readLevelFixture(t, project, "generated/public/"+ref.Path)
					for _, entry := range chunk.Bundle.Entries {
						if entry.SHA256 == ref.SHA256 {
							t.Fatal("material texture duplicated in chunk bundle")
						}
					}
				}
			}
		}
	}

	if len(uses) != 1 {
		t.Fatal("fixture changed shared material roster", uses)
	}

	for _, count := range uses {
		if count < 2 {
			t.Fatal("material fixture no longer exercises cross-chunk reuse")
		}
	}
}

func TestChunkBundlesRetainDevelopmentInputs(t *testing.T) {
	t.Parallel()
	project := writeFixture(t)
	config := fixtureConfig(fixtureChunkBundleBake())

	config.Pipelines[0].Output = []assetbuild.Plugin{Publish(PublishOptions{BundleChunks: true, Development: true})}
	if _, err := assetbuild.Build(
		t.Context(),
		config,
		assetbuild.Options{Project: project, Identity: "chunk-bundle-development"},
	); err != nil {
		t.Fatal(err)
	}

	for _, chunk := range findManifest(t, project).Chunks {
		for _, page := range chunk.Pages {
			if page.BakeInputs == nil {
				t.Fatal("development input missing")
			}

			_ = readLevelFixture(t, project, "generated/public/"+page.BakeInputs.Path)
			for _, entry := range chunk.Bundle.Entries {
				if entry.SHA256 == page.BakeInputs.SHA256 {
					t.Fatal("development payload bundled into runtime chunk")
				}
			}
		}
	}
}

func TestChunkZIPCapacityRejectsOversizeBeforeReadingPayloads(t *testing.T) {
	t.Parallel()

	maximum := leveldata.MaxPayloadBytes - storedZIPEndBytes - storedZIPEntryOverhead

	records := []fileRecord{{Reference: leveldata.Ref{Size: maximum}}}
	if size, err := chunkZIPCapacity(records); err != nil || size != leveldata.MaxPayloadBytes {
		t.Fatal(size, err)
	}

	for _, size := range []int{-1, 0, maximum + 1, leveldata.MaxPayloadBytes} {
		records[0].Reference.Size = size
		if _, err := chunkZIPCapacity(records); err == nil {
			t.Fatal("oversize or invalid bundle was admitted", size)
		}
	}

	records = []fileRecord{{Reference: leveldata.Ref{Size: maximum - 1}}, {Reference: leveldata.Ref{Size: 1}}}
	if _, err := chunkZIPCapacity(records); err == nil {
		t.Fatal("per-entry ZIP overhead escaped the complete-file limit")
	}
}
