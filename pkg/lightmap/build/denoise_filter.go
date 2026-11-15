//go:build !js

//nolint:mnd // The immutable receiver/frame ABI uses fixed byte offsets and three spatial axes.
package lightmapbuild

import (
	"context"
	"encoding/binary"
	"math"

	"github.com/karty-game/karty-webgpu/internal/lightmapdata"
)

type denoiseGuide struct {
	position, normal, tangent [3]float32
	chart, coverage, handed   float32
}

func denoiseGuideAt(document *lightmapdata.Document, pixel int) denoiseGuide {
	receiver := document.Receivers[pixel*lightmapdata.ReceiverStride:]
	frame := document.Frames[pixel*lightmapdata.FrameStride:]

	var guide denoiseGuide
	for axis := range 3 {
		guide.position[axis] = denoiseNumber(receiver[axis*4:])
		guide.normal[axis] = denoiseNumber(receiver[16+axis*4:])
		guide.tangent[axis] = denoiseNumber(frame[axis*4:])
	}

	guide.coverage, guide.chart, guide.handed = denoiseNumber(receiver[12:]), denoiseNumber(receiver[28:]), denoiseNumber(frame[12:])

	return guide
}

func denoiseNumber(data []byte) float32 {
	return math.Float32frombits(binary.LittleEndian.Uint32(data))
}

func denoisePass(
	ctx context.Context,
	document *lightmapdata.Document,
	links []byte,
	source, output []float32,
	options DenoiseOptions,
	step int,
) error {
	width, height := int(document.Width), int(document.Height)
	for pixelY := range height {
		if err := ctx.Err(); err != nil {
			return err
		}

		for pixelX := range width {
			pixel := pixelY*width + pixelX

			guide := denoiseGuideAt(document, pixel)
			if guide.coverage == 0 {
				continue
			}

			spacing := denoiseSpacing(document, links, guide, pixelX, pixelY)
			if spacing == 0 {
				copy(output[pixel*denoiseChannels:(pixel+1)*denoiseChannels], source[pixel*denoiseChannels:(pixel+1)*denoiseChannels])

				continue
			}

			denoisePixel(document, links, source, output, guide, pixel, spacing, options, step)
		}
	}

	return nil
}

func denoiseSpacing(document *lightmapdata.Document, links []byte, center denoiseGuide, pixelX, pixelY int) float32 {
	width, height := int(document.Width), int(document.Height)
	minimum := float32(math.MaxFloat32)

	for _, offset := range [][2]int{{-1, 0}, {1, 0}, {0, -1}, {0, 1}} {
		neighborX, neighborY := pixelX+offset[0], pixelY+offset[1]
		if neighborX < 0 || neighborX >= width || neighborY < 0 || neighborY >= height {
			continue
		}

		if !denoiseConnected(links, width, pixelX, pixelY, neighborX, neighborY) {
			continue
		}

		neighbor := denoiseGuideAt(document, neighborY*width+neighborX)

		distance, weight := denoiseGeometry(center, neighbor)
		if weight > 0 && distance > 1e-12 {
			minimum = min(minimum, distance)
		}
	}

	if minimum == math.MaxFloat32 {
		return 0
	}

	return minimum
}

func denoiseGeometry(center, neighbor denoiseGuide) (float32, float32) {
	if neighbor.coverage != center.coverage || neighbor.chart != center.chart || neighbor.handed != center.handed {
		return 0, 0
	}

	var normal, tangent, distance, plane float32
	for axis := range 3 {
		normal += center.normal[axis] * neighbor.normal[axis]
		tangent += center.tangent[axis] * neighbor.tangent[axis]
		delta := neighbor.position[axis] - center.position[axis]
		distance += delta * delta
		plane += delta * center.normal[axis]
	}
	// Keep coefficients in the same RNM frame. The plane test rejects parallel
	// walls even when their atlas chart metadata and shading frames match.
	if normal < .995 || tangent < .995 || plane*plane > distance*.0625+1e-12 || math.IsInf(float64(distance), 0) {
		return 0, 0
	}

	normal, tangent = min(normal, 1), min(tangent, 1)
	for range 3 {
		normal *= normal
		tangent *= tangent
	}

	return distance, normal * tangent
}

func denoisePixel(
	document *lightmapdata.Document,
	links []byte,
	source, output []float32,
	guide denoiseGuide,
	pixel int,
	spacing float32,
	options DenoiseOptions,
	step int,
) {
	width, height := int(document.Width), int(document.Height)
	pixelX, pixelY := pixel%width, pixel/width

	var sums [denoiseChannels]float64
	for channel := range denoiseChannels {
		sums[channel] = float64(source[pixel*denoiseChannels+channel])
	}

	weightSum := float64(1)
	worldStep := spacing * float32(step*step)

	limit := worldStep * float32(options.Radius*options.Radius) * 2.25
	for deltaY := -options.Radius; deltaY <= options.Radius; deltaY++ {
		for deltaX := -options.Radius; deltaX <= options.Radius; deltaX++ {
			neighborX, neighborY := pixelX+deltaX*step, pixelY+deltaY*step
			if deltaX == 0 && deltaY == 0 || neighborX < 0 || neighborX >= width || neighborY < 0 || neighborY >= height {
				continue
			}

			if !denoiseConnected(links, width, pixelX, pixelY, neighborX, neighborY) {
				continue
			}

			neighborPixel := neighborY*width + neighborX

			distance, geometry := denoiseGeometry(guide, denoiseGuideAt(document, neighborPixel))
			if geometry == 0 || distance > limit {
				continue
			}

			weight := float64(geometry / ((1 + float32(deltaX*deltaX+deltaY*deltaY)) * (1 + distance/worldStep)))

			weightSum += weight
			for channel := range denoiseChannels {
				sums[channel] += float64(source[neighborPixel*denoiseChannels+channel]) * weight
			}
		}
	}

	for channel := range denoiseChannels {
		index := pixel*denoiseChannels + channel
		output[index] = source[index] + (float32(sums[channel]/weightSum)-source[index])*options.Strength
	}
}
