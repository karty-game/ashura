//go:build !js

package build

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
	"strings"

	"github.com/karty-game/ashura/internal/texturedata"
	"github.com/karty-game/ashura/internal/uipaint"
)

type spriteRegion struct {
	uv     [4]float64
	aspect float64
}

const (
	builtinWidth = 64
	sparkHeight  = 16
	cellSize     = builtinWidth + 2
)

type particleSprite struct {
	name          string
	width, height int
	pixels        image.Image
}

func builtinHeight(texture string) int {
	switch texture {
	case "circle-soft", "circle-hard", "square", "smoke":
		return builtinWidth
	case "spark":
		return sparkHeight
	default:
		return 0
	}
}

// Pack only textures referenced by enabled layers, in first-use order.
func preparePage(doc document) (*uipaint.Texture, map[string]spriteRegion, error) {
	seen := make(map[string]bool)

	var sprites []particleSprite

	for _, layer := range doc.Layers {
		name := layer.Texture.Ref
		if !layer.Enabled || seen[name] {
			continue
		}

		sprite, err := prepareSprite(name, doc.Textures)
		if err != nil {
			return nil, nil, fmt.Errorf("particlr layer %q: %w", layer.ID, err)
		}

		if sprite.width != 0 {
			sprites = append(sprites, sprite)
		}

		seen[name] = true
	}

	page, regions := packPage(sprites)

	return page, regions, nil
}

func prepareSprite(name string, textures map[string]string) (particleSprite, error) {
	if height := builtinHeight(name); height != 0 {
		return particleSprite{name: name, width: builtinWidth, height: height}, nil
	}

	key, userTexture := strings.CutPrefix(name, "user:")

	embedded := textures[key]
	if !userTexture || embedded == "" {
		return particleSprite{}, nil
	}

	const (
		prefix      = "data:image/png;base64,"
		maxPNGBytes = 64 << 10
	)

	encoded, hasPrefix := strings.CutPrefix(embedded, prefix)
	if !hasPrefix || base64.StdEncoding.DecodedLen(len(encoded)) > maxPNGBytes {
		return particleSprite{}, fmt.Errorf("texture %q requires a PNG data URL of at most %d bytes: %w", name, maxPNGBytes, errUnsupported)
	}

	data, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return particleSprite{}, fmt.Errorf("texture %q: %w", name, err)
	}

	config, err := png.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return particleSprite{}, fmt.Errorf("texture %q: %w", name, err)
	}

	if config.Width < 1 || config.Height < 1 || config.Width > builtinWidth || config.Height > builtinWidth {
		return particleSprite{}, fmt.Errorf("texture %q exceeds %d pixels per side: %w", name, builtinWidth, errUnsupported)
	}

	pixels, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		return particleSprite{}, fmt.Errorf("texture %q: %w", name, err)
	}

	return particleSprite{name: name, width: config.Width, height: config.Height, pixels: pixels}, nil
}

func packPage(sprites []particleSprite) (*uipaint.Texture, map[string]spriteRegion) {
	const channels = 4

	width := max(cellSize, len(sprites)*cellSize)
	page := &uipaint.Texture{
		Width: width, Height: cellSize, Format: texturedata.RGBA8,
		Pixels: make([]byte, width*cellSize*channels),
	}
	regions := make(map[string]spriteRegion, len(sprites))

	for index, sprite := range sprites {
		left := index*cellSize + 1

		for row := -1; row <= sprite.height; row++ {
			for x := -1; x <= sprite.width; x++ {
				offset := ((row+1)*width + left + x) * channels
				pixel := sprite.texel(x, row)
				page.Pixels[offset], page.Pixels[offset+1] = pixel.R, pixel.G
				page.Pixels[offset+2], page.Pixels[offset+3] = pixel.B, pixel.A
			}
		}

		regions[sprite.name] = spriteRegion{
			uv: [4]float64{
				float64(left) / float64(width),
				1.0 / cellSize,
				float64(left+sprite.width) / float64(width),
				float64(sprite.height+1) / cellSize,
			},
			aspect: float64(sprite.height) / float64(sprite.width),
		}
	}

	return page, regions
}

func (sprite particleSprite) texel(column, row int) color.NRGBA {
	inside := column >= 0 && column < sprite.width && row >= 0 && row < sprite.height
	if sprite.pixels == nil {
		const white = 255

		texel := color.NRGBA{R: white, G: white, B: white}

		if inside {
			alpha := builtinAlpha(sprite.name, float64(column)+.5, float64(row)+.5)
			texel.A = byte(math.Round(min(alpha, 1) * white))
		}

		return texel
	}

	// Copy RGB from the edge into the transparent gutter for linear filtering.
	edge := sprite.pixels.At(min(max(column, 0), sprite.width-1), min(max(row, 0), sprite.height-1))

	texel := color.NRGBAModel.Convert(edge).(color.NRGBA) //nolint:errcheck,forcetypeassert // NRGBAModel always returns color.NRGBA.
	if !inside {
		texel.A = 0
	}

	return texel
}

// Built-in alpha functions follow the MIT particlr texture generator.
func builtinAlpha(name string, pixelX, pixelY float64) float64 {
	radius := math.Hypot(pixelX-32, pixelY-32)

	switch name {
	case "circle-soft":
		return square(max(0, 1-radius/32))
	case "circle-hard":
		const outerRadius, feather = 30, 2

		return min(1, max(0, (outerRadius-radius)/feather))
	case "square":
		if pixelX >= 8 && pixelX < 56 && pixelY >= 8 && pixelY < 56 {
			return 1
		}
	case "spark":
		return square(max(0, 1-math.Abs(pixelX-32)/32) * max(0, 1-math.Abs(pixelY-8)/8))
	case "smoke":
		blobs := [5][4]float64{{32, 32, 22, .55}, {24, 26, 14, .4}, {42, 30, 15, .4}, {30, 42, 13, .35}, {40, 40, 12, .3}}

		var alpha float64
		for _, blob := range blobs {
			alpha += blob[3] * square(max(0, 1-math.Hypot(pixelX-blob[0], pixelY-blob[1])/blob[2]))
		}

		return alpha
	}

	return 0
}

func square(value float64) float64 { return value * value }
