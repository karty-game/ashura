//go:build !js

package assetbuild

import (
	"context"
	"errors"
	"sync"

	"github.com/karty-game/ashura/internal/basiscodec"
	"github.com/karty-game/ashura/internal/ktx2data"
	"github.com/karty-game/ashura/internal/texturedata"
	"github.com/karty-game/ashura/pkg/texture"
)

const astcTileRows = 288
const astcSide = 6

type astcTile struct {
	level, width, first, count int
}

// Independent native blocks permit two bounded offline workers. Both retain
// the original operation deadline; only one extra codec instance is temporary.
// Tiles never cross a block row, and partial edges use the encoder's clamp rule.
func tiledASTC6x6(ctx context.Context, image texture.Data, quality int) ([][]byte, error) {
	bounded, cancel := context.WithCancel(ctx)
	defer cancel()

	levels, tiles := astcTilePlan(image)

	jobs := make(chan astcTile, len(tiles))
	for _, tile := range tiles {
		jobs <- tile
	}

	close(jobs)

	err := useBasis(bounded, func(shared *basiscodec.Session) error {
		extra := basiscodec.New()
		defer func() { _ = extra.Close() }()

		failures := make(chan error, 2)

		var workers sync.WaitGroup
		for _, session := range []*basiscodec.Session{shared, extra} {
			workers.Go(func() {
				for tile := range jobs {
					if err := encodeASTCTile(bounded, session, image, levels, tile, quality); err != nil {
						failures <- err

						cancel()

						return
					}
				}
			})
		}

		workers.Wait()
		close(failures)

		var joined error
		for failure := range failures {
			joined = errors.Join(joined, failure)
		}

		return joined
	})
	if err != nil {
		return nil, err
	}

	return levels, ctx.Err()
}

func astcTilePlan(image texture.Data) ([][]byte, []astcTile) {
	levels := make([][]byte, len(image.Levels))

	var tiles []astcTile

	for level := range image.Levels {
		width, height := max(1, image.Width>>level), max(1, image.Height>>level)
		row, rows := texturedata.Layout(texture.ASTC6x6, width, height)

		levels[level] = make([]byte, row*rows)
		for first := 0; first < height; first += astcTileRows {
			tiles = append(
				tiles,
				astcTile{level: level, width: width, first: first, count: min(astcTileRows, height-first)},
			)
		}
	}

	return levels, tiles
}

func encodeASTCTile(
	ctx context.Context,
	session *basiscodec.Session,
	image texture.Data,
	levels [][]byte,
	tile astcTile,
	quality int,
) error {
	width, height := texturedata.CopyExtent(texture.ASTC6x6, tile.width, tile.count)
	pixels := astcTilePixels(image.Levels[tile.level], tile, width, height)
	//nolint:gosec // The admitted source and six-texel padding bound dimensions below 4102.
	encoded, err := session.EncodeASTC6x6(ctx, basiscodec.Image{
		Width: uint32(width), Height: uint32(height), Levels: [][]byte{pixels}, SRGB: image.ColorSpace == texture.SRGB,
	}, quality)
	if err != nil {
		return err
	}

	native, err := canonicalASTC6x6(encoded, width, height, 1, image.ColorSpace)
	if err != nil {
		return err
	}

	data, err := ktx2data.Decode(native)
	if err != nil {
		return err
	}

	row, _ := texturedata.Layout(texture.ASTC6x6, tile.width, tile.count)
	copy(levels[tile.level][tile.first/astcSide*row:], data.Levels[0])

	return nil
}

func astcTilePixels(source []byte, tile astcTile, width, height int) []byte {
	const channels = texturedata.Channels
	if width == tile.width && height == tile.count {
		start := tile.first * tile.width * channels

		return source[start : start+width*height*channels]
	}

	pixels := make([]byte, width*height*channels)
	for y := range height {
		row := (tile.first + min(y, tile.count-1)) * tile.width
		for x := range width {
			from := (row + min(x, tile.width-1)) * channels
			copy(pixels[(y*width+x)*channels:], source[from:from+channels])
		}
	}

	return pixels
}
