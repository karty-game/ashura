//go:build !js

//nolint:errcheck,forcetypeassert // The maintained fixture's JSON schema is known.
package modelbuild_test

import (
	"bytes"
	"testing"

	"github.com/karty-game/ashura/internal/modelbuild"
	"github.com/karty-game/ashura/internal/modelbuild/testfixture"
	"github.com/karty-game/ashura/internal/modeldata"
	"github.com/karty-game/ashura/pkg/texture"
	"github.com/karty-game/ashura/pkg/texture/ktx2"
)

func TestTextureCapPreservesGeometryRolesAndSource(t *testing.T) {
	t.Parallel()

	document, data := testfixture.Triangle()
	data = testfixture.EmbedTexture(document, data)
	document["materials"].([]any)[0].(map[string]any)["normalTexture"] = map[string]any{"index": 0}
	source := testfixture.GLB(document, data)
	original := bytes.Clone(source)
	compile := func(maximum int) modeldata.Document {
		t.Helper()

		result, err := modelbuild.CompileWithOptions(source, "fixture.glb", modelbuild.Options{MaxTextureDimension: maximum})
		if err != nil {
			t.Fatal(err)
		}

		prepared, err := modeldata.Decode(result.Data)
		if err != nil {
			t.Fatal(err)
		}

		return prepared
	}

	full, small := compile(0), compile(1)
	if !bytes.Equal(source, original) || !bytes.Equal(full.Parts[0].Vertices, small.Parts[0].Vertices) ||
		!bytes.Equal(full.Parts[0].Indices, small.Parts[0].Indices) ||
		full.Parts[0].Material != small.Parts[0].Material {
		t.Fatal("texture maximum changed source, geometry or material bindings")
	}

	if len(small.Textures) != 2 {
		t.Fatal("missing texture roles")
	}

	for index, image := range small.Textures {
		decoded, err := ktx2.Decode(image.Data)
		if err != nil {
			t.Fatal(err)
		}

		expected := texture.SRGB
		if index == 1 {
			expected = texture.Linear
		}

		if decoded.Width != 1 || decoded.Height != 1 || len(decoded.Levels) != 1 || decoded.ColorSpace != expected ||
			image.Role != full.Textures[index].Role {
			t.Fatal("texture maximum lost dimensions, mips or color interpretation")
		}
	}

	for _, maximum := range []int{-1, 4097} {
		if _, err := modelbuild.CompileWithOptions(source, "fixture.glb", modelbuild.Options{MaxTextureDimension: maximum}); err == nil {
			t.Fatal("invalid texture maximum accepted", maximum)
		}
	}
}
