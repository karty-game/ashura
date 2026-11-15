//go:build !js

package assetbuild

import (
	"math"

	"github.com/karty-game/karty-webgpu/internal/texturedata"
)

const (
	portableBlockSide   = 4
	portableBlockPixels = 16
)

// portableRGBEBlocks aligns each 4x4 block's mantissas to its largest exponent.
// Exponents are stored separately for exact constant-alpha EAC blocks. Opaque
// alpha in the color input prevents a lossy intermediate from spending bits on
// numeric exponents. Black source pixels remain black before color compression.
func portableRGBEBlocks(image texturedata.Data) ([]byte, []byte) {
	source := image.Levels[0]
	pixels := make([]byte, len(source))
	exponents := make([]byte, image.Width*image.Height/portableBlockPixels)
	block := 0

	for pixelY := 0; pixelY < image.Height; pixelY += portableBlockSide {
		for pixelX := 0; pixelX < image.Width; pixelX += portableBlockSide {
			exponent := portableBlockExponent(source, image.Width, pixelX, pixelY)
			exponents[block] = exponent
			block++

			portableBlockMantissas(source, pixels, image.Width, pixelX, pixelY, exponent)
		}
	}

	return pixels, exponents
}

func portableBlockExponent(source []byte, width, pixelX, pixelY int) byte {
	exponent := byte(0)

	for deltaY := range portableBlockSide {
		for deltaX := range portableBlockSide {
			offset := ((pixelY+deltaY)*width + pixelX + deltaX) * texturedata.Channels
			if source[offset] != 0 || source[offset+1] != 0 || source[offset+2] != 0 {
				exponent = max(exponent, source[offset+3])
			}
		}
	}

	return exponent
}

func portableBlockMantissas(source, pixels []byte, width, pixelX, pixelY int, exponent byte) {
	for deltaY := range portableBlockSide {
		for deltaX := range portableBlockSide {
			offset := ((pixelY+deltaY)*width + pixelX + deltaX) * texturedata.Channels

			factor := float64(0)
			if source[offset+3] != 0 && exponent != 0 {
				factor = math.Ldexp(1, int(source[offset+3])-int(exponent))
			}

			for channel := range 3 {
				pixels[offset+channel] = byte(min(255, math.RoundToEven(float64(source[offset+channel])*factor)))
			}

			pixels[offset+3] = 255
		}
	}
}
