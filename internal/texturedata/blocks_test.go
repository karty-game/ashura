package texturedata

import "testing"

func TestASTC6x6PhysicalMipLayout(t *testing.T) {
	t.Parallel()

	data := Data{Width: 18, Height: 18, Format: ASTC6x6,
		Levels: [][]byte{make([]byte, 144), make([]byte, 64), make([]byte, 16), make([]byte, 16), make([]byte, 16)}}

	image, err := FromOwned(data)
	if err != nil || ByteSize(image) != 256 {
		t.Fatal("ASTC physical mip lengths", image, err)
	}

	for _, size := range []struct{ logical, physical int }{{18, 18}, {9, 12}, {4, 6}, {2, 6}, {1, 6}} {
		width, height := CopyExtent(ASTC6x6, size.logical, size.logical)
		if width != size.physical || height != size.physical {
			t.Fatal("ASTC physical mip extent", size, width, height)
		}
	}

	data.Levels[1] = make([]byte, 16)
	if _, err := FromOwned(data); err == nil {
		t.Fatal("admitted truncated ASTC mip")
	}

	data.Width = 16
	if _, err := FromOwned(data); err == nil {
		t.Fatal("admitted ASTC base aligned to four instead of six")
	}
}

func TestCompressedMipBoundsAndOwnership(t *testing.T) {
	t.Parallel()

	for _, format := range []Format{BC1, ETC2RGB} {
		data := Data{
			Width:      8,
			Height:     8,
			ColorSpace: SRGB,
			Format:     format,
			Levels:     [][]byte{make([]byte, 32), make([]byte, 8), make([]byte, 8), make([]byte, 8)},
		}

		image, err := FromOwned(data)
		if err != nil {
			t.Fatal(err)
		}

		if ByteSize(image) != 56 || &Levels(image)[0][0] != &data.Levels[0][0] {
			t.Fatal("compressed payload copied or incorrectly counted")
		}

		data.Levels[3] = make([]byte, 4)
		if _, err := Create(data); err == nil {
			t.Fatal("accepted RGBA-sized compressed mip tail")
		}

		data.Width = 7
		if _, err := Create(data); err == nil {
			t.Fatal("accepted unaligned compressed base size")
		}
	}
}

func TestNonPowerOfTwoCompressedMipBoundsAndExtent(t *testing.T) {
	t.Parallel()

	for _, format := range []Format{BC1, ETC2RGB} {
		data := Data{
			Width:      12,
			Height:     12,
			ColorSpace: SRGB,
			Format:     format,
			Levels:     [][]byte{make([]byte, 72), make([]byte, 32), make([]byte, 8), make([]byte, 8)},
		}

		image, err := FromOwned(data)
		if err != nil {
			t.Fatal(err)
		}

		if ByteSize(image) != 120 {
			t.Fatalf("non-power-of-two compressed mip bytes = %d, want 120", ByteSize(image))
		}

		for _, size := range []struct{ logical, physical int }{{12, 12}, {6, 8}, {3, 4}, {1, 4}} {
			width, height := CopyExtent(format, size.logical, size.logical)
			if width != size.physical || height != size.physical {
				t.Fatalf("format %d, logical mip %d: physical extent %dx%d, want %dx%d",
					format, size.logical, width, height, size.physical, size.physical)
			}
		}

		// A 6x6 mip requires four complete blocks, rather than one mip-tail block.
		data.Levels[1] = make([]byte, 8)
		if _, err := FromOwned(data); err == nil {
			t.Fatal("accepted truncated 6x6 compressed mip")
		}
	}

	if width, height := CopyExtent(RGBA8, 6, 3); width != 6 || height != 3 {
		t.Fatalf("RGBA extent was block-rounded: %dx%d", width, height)
	}
}
