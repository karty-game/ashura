//go:build !js

package build

import (
	"encoding/binary"
	"errors"
	"math"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestDefaultFontPublicationAdmitsExistingPairAndRejectsCorruption(t *testing.T) {
	t.Parallel()

	metrics, err := os.ReadFile(filepath.Join("..", "fonts", "defaultfont", "prepared", "glyphs.bin"))
	if err != nil {
		t.Fatal(err)
	}

	atlas, err := os.ReadFile(filepath.Join("..", "fonts", "defaultfont", "prepared", "atlas.rgba"))
	if err != nil {
		t.Fatal(err)
	}

	if err := validateDefaultFont(metrics, atlas); err != nil {
		t.Fatal("the retained qualified font pair must remain valid", err)
	}

	for _, corrupt := range []func([]byte, []byte){
		func(_ []byte, page []byte) { page[0] ^= 1 },
		func(metadata, _ []byte) { metadata[28] ^= 1 },
		func(metadata, _ []byte) { binary.LittleEndian.PutUint32(metadata[24:], math.MaxUint32) },
		func(metadata, _ []byte) {
			binary.LittleEndian.PutUint32(metadata[fontHeaderBytes+4:], math.Float32bits(float32(math.NaN())))
		},
		func(metadata, _ []byte) {
			binary.LittleEndian.PutUint32(metadata[fontHeaderBytes+20:], math.Float32bits(2))
		},
	} {
		metadata, page := slices.Clone(metrics), slices.Clone(atlas)
		corrupt(metadata, page)

		if err := validateDefaultFont(metadata, page); !errors.Is(err, errDefaultFont) {
			t.Fatal("corrupt or mismatched output must fail before publication", err)
		}
	}
}
