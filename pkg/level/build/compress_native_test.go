//go:build !js

package levelbuild

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"testing"

	"github.com/karty-game/karty-webgpu/internal/leveldata"
	"github.com/karty-game/karty-webgpu/pkg/assetbuild"
)

func TestNativeBlockCollectionSeparatesAliasedTextureRoles(t *testing.T) {
	t.Parallel()

	data, err := assetbuild.EncodeTexture(assetbuild.Image{
		Name: "Shared", Source: "shared.png", Usage: assetbuild.UsageSSBump,
		Width: 8, Height: 8, Levels: [][]byte{bytes.Repeat([]byte{128, 64, 32, 128}, 8*8)},
	})
	if err != nil {
		t.Fatal(err)
	}

	digest := fmt.Sprintf("%x", sha256.Sum256(data))
	reference := leveldata.Ref{Path: "levels/textures/" + digest + ".ktx2", Size: len(data), SHA256: digest}
	file := fileRecord{Reference: reference, Artifact: assetbuild.Artifact{SHA256: digest, Size: len(data)}}
	page := publishedPage{
		Page: leveldata.Page{
			Maps:      [3]leveldata.Ref{reference, reference, reference},
			Materials: []leveldata.Material{{Name: "first", SSBump: &reference}, {Name: "second", SSBump: &reference}},
		},
		Files: []fileRecord{file},
	}
	inputs := nativeBlockInputs{files: map[string]fileRecord{}, lightmaps: map[string]bool{}}
	collectNativePageFiles(page, inputs.files, inputs.lightmaps)

	rnmKey, ssbumpKey := nativeBlockKey(nativeRNMRole, digest), nativeBlockKey(nativeSSBumpRole, digest)
	if len(inputs.files) != 2 || inputs.files[rnmKey] != file || inputs.files[ssbumpKey] != file {
		t.Fatalf("aliased input must retain one job for each role: %#v", inputs.files)
	}

	if !inputs.lightmaps[rnmKey] || inputs.lightmaps[ssbumpKey] {
		t.Fatalf("aliased input selected the wrong codec: %#v", inputs.lightmaps)
	}

	var plan assetbuild.Plan

	encoded, dependencies, err := nativeBlockActions(&plan, "aliased", inputs, NativeHDROptions{}, nil)
	if err != nil {
		t.Fatal(err)
	}

	if len(encoded) != 2 || len(dependencies) != 2 || encoded[rnmKey] == encoded[ssbumpKey] {
		t.Fatalf("aliased input must register two independent encoding actions: %#v %#v", encoded, dependencies)
	}

	if encoded[rnmKey].Action != "level/native-block/aliased/"+rnmKey ||
		encoded[ssbumpKey].Action != "level/native-block/aliased/"+ssbumpKey {
		t.Fatal("action identities must include the encoding role")
	}
}

func TestNativeBlockReplacementKeepsBothEncodedRoles(t *testing.T) {
	t.Parallel()

	raw := fileRecord{Reference: leveldata.Ref{SHA256: "shared-source"}}
	rnm := fileRecord{Reference: leveldata.Ref{SHA256: "native-rnm"}}
	ssbump := fileRecord{Reference: leveldata.Ref{SHA256: "native-ssbump"}}

	files := withoutPublishedFiles([]fileRecord{raw, rnm, ssbump}, map[string]bool{raw.Reference.SHA256: true})
	if len(files) != 2 || files[0] != rnm || files[1] != ssbump {
		t.Fatal("removing shared source must retain both encoded roles")
	}
}
