//go:build !js

//nolint:mnd,gosec // Pinned C API identifiers and wasm32 return words.
package basiscodec

import (
	"context"
	"math"
)

// Result owns the requested native GPU block payloads and their original mips.
type Result struct {
	Width, Height uint32
	SRGB          bool
	Levels        [][]byte
}

// Transcode accepts a previously admitted KTX2 file and a C API target number.
// The caller retains complete profile/container admission before this call.
func (session *Session) Transcode(ctx context.Context, data []byte, target uint64) (result Result, err error) {
	if len(data) < 80 || len(data) > 128<<20 || !supportedTarget(target) {
		return result, ErrInvalid
	}

	err = session.run(ctx, func() error {
		guest := session.guest(ctx, false)

		address := guest.copy(data)
		defer guest.release("free", address)

		handle := guest.call("ktx2_open", address, uint64(len(data)))
		if handle == 0 {
			return ErrInvalid
		}
		defer guest.release("ktx2_close", handle)

		result.Width, result.Height = uint32(guest.call("ktx2_get_width", handle)), uint32(guest.call("ktx2_get_height", handle))
		count := guest.call("ktx2_get_levels", handle)

		result.SRGB = guest.call("ktx2_is_srgb", handle) != 0
		if !validExtent(result.Width, result.Height, count) ||
			guest.call("ktx2_get_faces", handle) != 1 ||
			guest.call("ktx2_get_layers", handle) > 1 ||
			guest.call("ktx2_start_transcoding", handle) == 0 {
			return ErrInvalid
		}

		var total uint64

		for index := range count {
			width, height := max(1, result.Width>>index), max(1, result.Height>>index)
			size := guest.call("basis_compute_transcoded_image_size_in_bytes", target, uint64(width), uint64(height))
			blockBytes := guest.call("basis_get_bytes_per_block_or_pixel", target)

			total += size
			if size == 0 || total > 128<<20 || blockBytes == 0 || size%blockBytes != 0 {
				return ErrInvalid
			}

			result.Levels = append(result.Levels, guest.transcodeLevel(handle, index, target, size, blockBytes))
		}

		return nil
	})
	if err != nil {
		return Result{}, err
	}

	return result, nil
}

func supportedTarget(target uint64) bool {
	switch target {
	case 0, 1, 2, 6, 10, 13, 22:
		return true
	default:
		return false
	}
}

func validExtent(width, height uint32, count uint64) bool {
	return width > 0 && height > 0 && width <= 16384 && height <= 16384 && count > 0 && count <= 15
}

func (guest guest) transcodeLevel(handle, index, target, size, blockBytes uint64) []byte {
	output := guest.allocate(size)

	accepted := guest.call(
		"ktx2_transcode_image_level",
		handle,
		index,
		0,
		0,
		output,
		size/blockBytes,
		target,
		0,
		0,
		0,
		math.MaxUint32,
		math.MaxUint32,
		0,
	)
	if accepted == 0 {
		panic(ErrInvalid)
	}

	level := guest.read(output, size)
	guest.call("free", output)

	return level
}
