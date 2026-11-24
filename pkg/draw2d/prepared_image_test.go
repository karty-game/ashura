package draw2d

import (
	"errors"
	"testing"

	"github.com/karty-game/ashura/pkg/texture"
)

func TestCompressedImageAdmissionOwnsBlocksAndSharesRegions(t *testing.T) {
	t.Parallel()

	blocks := make([]byte, 32)
	blocks[0] = 31

	image, err := NewPreparedImage(8, 4, texture.BC7, blocks)
	if err != nil {
		t.Fatal(err)
	}

	blocks[0] = 0

	region, err := image.Region(1, 1, 3, 2)
	if err != nil {
		t.Fatal(err)
	}

	if region.page != image.page || image.page.Format != texture.BC7 || image.page.Pixels[0] != 31 || len(image.page.Pixels) != 32 {
		t.Fatal("compressed regions lost their owned page or format")
	}

	for _, test := range []struct {
		width, height int
		format        texture.Format
		bytes         int
	}{
		{8, 4, texture.BC7, 31},
		{6, 4, texture.BC7, 32},
		{4, 4, texture.BC6H, 16},
		{4, 4, texture.Format(255), 64},
		{4097, 4, texture.BC1, 8},
	} {
		if _, err := NewPreparedImage(test.width, test.height, test.format, make([]byte, test.bytes)); !errors.Is(err, ErrImage) {
			t.Errorf("invalid prepared page admitted: %+v: %v", test, err)
		}
	}
}
