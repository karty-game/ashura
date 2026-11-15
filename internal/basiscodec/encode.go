//go:build !js

//nolint:mnd,gosec // Pinned C API flags; quality is admitted to 0..255 before conversion.
package basiscodec

import (
	"context"
	"math"
)

// Image contains authored mip bytes, largest first. HDR levels are RGBA32F LE;
// LDR levels are RGBA8. No mip generation, channel packing or resizing occurs.
type Image struct {
	Width, Height uint32
	Levels        [][]byte
	HDR, SRGB     bool
}

func (image Image) validate() bool {
	if image.Width == 0 || image.Height == 0 || image.Width > 16384 || image.Height > 16384 || len(image.Levels) == 0 ||
		len(image.Levels) > 15 ||
		image.HDR && (image.SRGB || len(image.Levels) != 1) {
		return false
	}

	bpp, width, height, total := uint64(4), image.Width, image.Height, uint64(0)
	if image.HDR {
		bpp = 16
	}

	for index, level := range image.Levels {
		if index > 0 && width == 1 && height == 1 && len(image.Levels[index-1]) == int(bpp) {
			return false
		}

		if uint64(len(level)) != uint64(width)*uint64(height)*bpp {
			return false
		}

		total += uint64(len(level))
		width, height = max(1, width>>1), max(1, height>>1)
	}

	return total <= 128<<20
}

// Encode creates a KTX2 UASTC LDR/HDR intermediate. ETC1S explicitly selects
// its 1..255 codebook quality; other profiles accept search effort 0..4.
func (session *Session) Encode(ctx context.Context, image Image, quality int, etc1s bool) (output []byte, err error) {
	if !image.validate() || etc1s && (image.HDR || quality < 1 || quality > 255) || !etc1s && (quality < 0 || quality > 4) {
		return nil, ErrInvalid
	}

	err = session.run(ctx, func() error {
		guest := session.guest(ctx, true)

		parameters := guest.call("new_comp_params")
		if parameters == 0 {
			return ErrInvalid
		}
		defer guest.release("delete_comp_params", parameters)

		for index, data := range image.Levels {
			address := guest.copy(data)

			name, bpp := "comp_params_set_image_rgba32", uint64(4)
			if image.HDR {
				name, bpp = "comp_params_set_image_float_rgba", 16
			}

			accepted := guest.call(
				name,
				parameters,
				uint64(index),
				address,
				uint64(max(1, image.Width>>index)),
				uint64(max(1, image.Height>>index)),
				uint64(max(1, image.Width>>index))*bpp,
			)
			guest.call("free", address)

			if accepted == 0 {
				return ErrInvalid
			}
		}

		format, flags := uint64(1), uint64(1<<11)|uint64(quality)
		if image.HDR {
			format = 2
		}

		if etc1s {
			format = 0
		}

		if image.SRGB {
			flags |= 1 << 13
		}

		if guest.call("compress_texture", parameters, format, math.MaxUint32, math.MaxUint32, flags, 0) == 0 {
			return ErrInvalid
		}

		output = guest.read(
			guest.call("comp_params_get_comp_data_ofs", parameters),
			guest.call("comp_params_get_comp_data_size", parameters),
		)

		return nil
	})
	if err != nil {
		return nil, err
	}

	return output, nil
}
