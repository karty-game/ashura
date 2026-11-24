//go:build !js

package levelbuild

import (
	"context"
	"crypto/sha256"
	"fmt"
	"image"
	"maps"
	"math"
	"slices"

	"github.com/karty-game/ashura/internal/ktx2data"
	"github.com/karty-game/ashura/internal/modeldata"
	"github.com/karty-game/ashura/internal/ssbump"
	"github.com/karty-game/ashura/internal/texturedata"
	"github.com/karty-game/ashura/pkg/assetbuild"
)

const (
	sourceNormalLimit    = 512
	sourceNormalMinimum  = 2
	sourceNormalMidpoint = .5
	sourceNormalRays     = 128
	sourceNormalDistance = 16
	sourcePixelBytes     = 4
	sourceByteMaximum    = 255
)

type sourceNormalKey struct {
	digest   [sha256.Size]byte
	strength float32
}

// autoSSBump preserves authored per-primitive overrides and derives missing
// Source coefficients from prepared tangent normals offline. The input model
// is unchanged; normal strength is applied before projection, exactly once.
func autoSSBump(
	ctx *assetbuild.Context,
	doc modeldata.Document,
	explicit map[string]assetbuild.Artifact,
) (map[string]assetbuild.Artifact, error) {
	if ctx == nil {
		return nil, ErrInvalid
	}

	if err := validateSourceOverrides(doc, explicit); err != nil {
		return nil, err
	}

	result := maps.Clone(explicit)
	if result == nil {
		result = make(map[string]assetbuild.Artifact)
	}

	cache := make(map[sourceNormalKey]assetbuild.Artifact)

	for _, part := range doc.Parts {
		if _, authored := explicit[part.Name]; authored || part.Material.Normal < 0 {
			continue
		}

		if int64(part.Material.Normal) >= int64(len(doc.Textures)) || math.IsNaN(float64(part.Material.NormalStrength)) ||
			math.IsInf(float64(part.Material.NormalStrength), 0) {
			return nil, fmt.Errorf("source normal %s has invalid material settings: %w", part.Name, ErrInvalid)
		}

		normal := doc.Textures[part.Material.Normal]
		if normal.Role != modeldata.Normal {
			return nil, fmt.Errorf("source normal %s has wrong texture role: %w", part.Name, ErrInvalid)
		}

		key := sourceNormalKey{digest: sha256.Sum256(normal.Data), strength: part.Material.NormalStrength}

		artifact, found := cache[key]
		if !found {
			data, err := bakeSourceSSBump(ctx, normal.Data, key.strength)
			if err != nil {
				return nil, fmt.Errorf("source normal %s: %w", part.Name, err)
			}

			artifact, err = ctx.StoreArtifact(data)
			if err != nil {
				return nil, err
			}

			cache[key] = artifact
		}

		if previous, duplicate := result[part.Name]; duplicate && previous != artifact {
			return nil, fmt.Errorf("source primitives named %s require incompatible normal maps: %w", part.Name, ErrInvalid)
		}

		result[part.Name] = artifact
	}

	return result, nil
}

func validateSourceOverrides(doc modeldata.Document, explicit map[string]assetbuild.Artifact) error {
	names := make(map[string]bool, len(doc.Parts))
	for _, part := range doc.Parts {
		names[part.Name] = true
	}

	for name := range explicit {
		if !names[name] {
			return fmt.Errorf("source SSBump override references unknown primitive %q: %w", name, ErrInvalid)
		}
	}

	return nil
}

func bakeSourceSSBump(ctx context.Context, encoded []byte, strength float32) ([]byte, error) {
	normal, err := sourceNormalImage(encoded, strength)
	if err != nil {
		return nil, err
	}

	prepared, err := ssbump.Generate(ctx, normal, ssbump.Options{Rays: sourceNormalRays, Distance: sourceNormalDistance})
	if err != nil {
		return nil, err
	}

	value, err := assetbuild.GenerateMipmaps(assetbuild.Image{
		Name: "SourceSSBump", Source: "source-ssbump.png", Usage: assetbuild.UsageSSBump,
		Width: prepared.Bounds().Dx(), Height: prepared.Bounds().Dy(), Levels: [][]byte{prepared.Pix},
	})
	if err != nil {
		return nil, err
	}

	return assetbuild.EncodeTexture(value)
}

func sourceNormalImage(encoded []byte, strength float32) (*image.NRGBA, error) {
	data, err := ktx2data.Decode(encoded)
	if err != nil {
		return nil, err
	}

	if data.ColorSpace != texturedata.Linear || math.IsNaN(float64(strength)) || math.IsInf(float64(strength), 0) {
		return nil, fmt.Errorf("source normals require finite strength and linear data: %w", ErrInvalid)
	}

	if data.Width != data.Height || data.Width&(data.Width-1) != 0 {
		return nil, fmt.Errorf("automatic SSBump requires square power-of-two normals; supply an explicit SSBump override: %w", ErrInvalid)
	}

	width, pixels, err := sourceNormalMip(data)
	if err != nil {
		return nil, err
	}

	pixels = slices.Clone(pixels)
	for offset := 0; offset < len(pixels); offset += sourcePixelBytes {
		normal := [3]float64{}
		for axis := range normal {
			normal[axis] = float64(pixels[offset+axis])*2/sourceByteMaximum - 1
		}

		normal[0] *= float64(strength)

		normal[1] *= float64(strength)
		if strength == 0 {
			normal[2] = 1
		}

		length := math.Hypot(math.Hypot(normal[0], normal[1]), normal[2])
		for axis := range normal {
			pixels[offset+axis] = byte(math.Round((normal[axis]/length*sourceNormalMidpoint + sourceNormalMidpoint) * sourceByteMaximum))
		}

		pixels[offset+sourcePixelBytes-1] = sourceByteMaximum
	}

	return &image.NRGBA{Pix: pixels, Stride: width * sourcePixelBytes, Rect: image.Rect(0, 0, width, width)}, nil
}

func sourceNormalMip(data texturedata.Data) (int, []byte, error) {
	if data.Width == 1 {
		pixels := make([]byte, sourceNormalMinimum*sourceNormalMinimum*sourcePixelBytes)
		for offset := 0; offset < len(pixels); offset += sourcePixelBytes {
			copy(pixels[offset:], data.Levels[0])
		}

		return sourceNormalMinimum, pixels, nil
	}

	width := data.Width
	for _, pixels := range data.Levels {
		if width <= sourceNormalLimit {
			return width, pixels, nil
		}

		width = max(1, width/2)
	}

	prepared, err := assetbuild.GenerateMipmaps(
		assetbuild.Image{Name: "SourceNormal", Source: "source-normal.png", Usage: assetbuild.UsageNormal,
			Width: data.Width, Height: data.Height, Levels: [][]byte{data.Levels[0]}},
	)
	if err != nil {
		return 0, nil, err
	}

	return sourceNormalMip(
		texturedata.Data{Width: prepared.Width, Height: prepared.Height, ColorSpace: texturedata.Linear, Levels: prepared.Levels},
	)
}
