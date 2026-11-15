//go:build !js

package impostorbake

import "context"

// PackBC1NormalDepth converts a completed RGBA normal/depth mip to octahedral
// normal R/B, depth G and opaque alpha. Pack after normal-aware mip generation;
// the depth uses RGB565's six-bit green endpoints. This is build-only work.
func PackBC1NormalDepth(ctx context.Context, pixels []byte) ([]byte, error) {
	if ctx == nil || len(pixels)%channels != 0 || len(pixels) > maxDimension*maxDimension*channels {
		return nil, errBounds
	}

	if err := ctx.Err(); err != nil {
		return nil, err
	}

	packed := make([]byte, len(pixels))

	const cancellationStride = 4096 * channels

	for offset := 0; offset < len(pixels); offset += channels {
		if offset%cancellationStride == 0 {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
		}

		pixel := [channels]byte(pixels[offset : offset+channels])
		normal := vector{
			float64(pixel[0])*2/byteMaximum - 1,
			float64(pixel[1])*2/byteMaximum - 1,
			float64(pixel[2])*2/byteMaximum - 1,
		}
		x, y := octNormal(normal)
		copy(packed[offset:offset+channels], []byte{x, pixel[3], y, byteMaximum})
	}

	return packed, nil
}
