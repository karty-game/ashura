//go:build !js

package assetbuild

import (
	"encoding/json"
	"errors"
	"fmt"
	"go/token"
	"io/fs"
	"path"
	"strings"
	"unicode"
)

const (
	// ImageSetSchema identifies immutable role-aware RGBA8 image records.
	ImageSetSchema = "karty.images@1"
	// PreparedSetSchema identifies GPU-ready KTX2 images and atlas mappings.
	PreparedSetSchema = "karty.prepared-images@1"
	// UsageAlbedo selects sRGB filtering and GPU encoding.
	UsageAlbedo = "albedo"
	// UsageNormal selects normalized tangent-space normal filtering.
	UsageNormal = "normal"
	// UsageHeight contains authored linear grayscale relief for offline SSBump baking.
	UsageHeight = "height"
	// UsageAO selects linear occlusion filtering.
	UsageAO = "ao"
	// UsageSSBump selects linear RNM coefficient filtering.
	UsageSSBump = "ssbump"
	// UsageLightmap selects linear RGB irradiance. Chart-safe mip limits belong
	// to the bake producer; callers must not generate unrestricted atlas mips.
	UsageLightmap = "lightmap"
	// UsageReflection selects an sRGB equirectangular source for probe baking.
	UsageReflection        = "reflection"
	maxImageDimension      = 4096
	maxImageBytes          = 96 << 20
	imageChannels          = 4
	authoringJPEGExtension = ".jpeg"
	imageDataSlot          = "data"
	imageCatalogSlot       = "catalog"
	imageFilesSlot         = "files"
)

var errImageRecord = errors.New("invalid prepared image record")

// ImageSet is one logical standalone image or an explicitly grouped atlas.
type ImageSet struct {
	Images  []Image       `json:"images"`
	Regions []AtlasRegion `json:"regions,omitempty"`
}

// Image contains tightly packed RGBA8 levels, largest first. MaxMipLevels zero
// permits a full chain; atlases can cap it to prevent cross-region filtering.
type Image struct {
	Name         string   `json:"name"`
	Source       string   `json:"source"`
	Usage        string   `json:"usage"`
	Width        int      `json:"width"`
	Height       int      `json:"height"`
	Levels       [][]byte `json:"levels"`
	MaxMipLevels int      `json:"maxMipLevels,omitempty"`
	Atlas        bool     `json:"atlas,omitempty"`
}

// AtlasRegion preserves a source image's location within a named atlas page.
type AtlasRegion struct {
	Name       string `json:"name"`
	Source     string `json:"source"`
	Page       string `json:"page"`
	X          int    `json:"x"`
	Y          int    `json:"y"`
	Width      int    `json:"width"`
	Height     int    `json:"height"`
	PageWidth  int    `json:"pageWidth"`
	PageHeight int    `json:"pageHeight"`
}

// PreparedSet contains encoded texture payloads and their resource mappings.
type PreparedSet struct {
	Images  []PreparedImage `json:"images"`
	Regions []AtlasRegion   `json:"regions,omitempty"`
}

// PreparedImage is an explicitly named KTX2 payload; Output is relative to the
// generated directory, never a runtime source-image lookup.
type PreparedImage struct {
	Name   string `json:"name"`
	Source string `json:"source"`
	Usage  string `json:"usage"`
	Output string `json:"output"`
	Data   []byte `json:"data,omitempty"`
}

// DecodeImageSet validates and decodes a processor's immutable image result.
func DecodeImageSet(data []byte) (ImageSet, error) {
	var set ImageSet
	if err := json.Unmarshal(data, &set); err != nil {
		return set, fmt.Errorf("decode images: %w", err)
	}

	if err := validateImageSet(set); err != nil {
		return ImageSet{}, err
	}

	return set, nil
}

// EncodeImageSet validates and serializes immutable image results.
func EncodeImageSet(set ImageSet) ([]byte, error) {
	if err := validateImageSet(set); err != nil {
		return nil, err
	}

	return json.Marshal(set)
}

func validateImageSet(set ImageSet) error {
	names := make(map[string]Image)
	regions := make(map[string]bool)

	var total int

	for _, value := range set.Images {
		if _, exists := names[value.Name]; exists {
			return fmt.Errorf("duplicate image %q: %w", value.Name, errImageRecord)
		}

		count, err := validateImage(value)
		if err != nil {
			return err
		}

		total += count
		if total > maxImageBytes {
			return fmt.Errorf("image group exceeds byte bounds: %w", errImageRecord)
		}

		names[value.Name] = value
	}

	for _, region := range set.Regions {
		page, ok := names[region.Page]
		if !ok || regions[region.Name] || !validRegion(region, page) {
			return fmt.Errorf("region %q bounds: %w", region.Name, errImageRecord)
		}

		regions[region.Name] = true
	}

	return nil
}

func validateImage(value Image) (int, error) {
	if value.Atlas && value.MaxMipLevels < 1 {
		return 0, fmt.Errorf("atlas image %q requires a safe positive mip cap: %w", value.Name, errImageRecord)
	}

	if !validImageMetadata(value) {
		return 0, fmt.Errorf("image %q metadata: %w", value.Name, errImageRecord)
	}

	width, height, total := value.Width, value.Height, 0
	for _, pixels := range value.Levels {
		if width == 0 || height == 0 || len(pixels) != width*height*imageChannels {
			return 0, fmt.Errorf("image %q mip dimensions: %w", value.Name, errImageRecord)
		}

		total += len(pixels)

		if width == 1 && height == 1 {
			width, height = 0, 0
		} else {
			width, height = max(1, width>>1), max(1, height>>1)
		}
	}

	return total, nil
}

func validImageMetadata(value Image) bool {
	return token.IsIdentifier(value.Name) && validUsage(value.Usage) && fs.ValidPath(value.Source) && value.Source != "." &&
		value.Width >= 1 && value.Height >= 1 && value.Width <= maxImageDimension && value.Height <= maxImageDimension &&
		len(value.Levels) > 0 && value.MaxMipLevels >= 0 &&
		(value.MaxMipLevels == 0 || len(value.Levels) <= value.MaxMipLevels)
}

func validRegion(region AtlasRegion, page Image) bool {
	return page.Atlas && token.IsIdentifier(region.Name) && region.X >= 0 && region.Y >= 0 &&
		region.Width >= 1 && region.Height >= 1 && region.PageWidth == page.Width && region.PageHeight == page.Height &&
		region.X <= page.Width-region.Width && region.Y <= page.Height-region.Height
}

func validUsage(usage string) bool {
	return usage == UsageAlbedo || usage == UsageNormal || usage == UsageHeight || usage == UsageAO || usage == UsageSSBump ||
		usage == UsageReflection ||
		usage == UsageLightmap
}

// AssetName converts a source-relative identity to a deterministic Go symbol.
func AssetName(stem string) string {
	parts := strings.FieldsFunc(stem, func(value rune) bool { return !unicode.IsLetter(value) && !unicode.IsDigit(value) })

	var name strings.Builder

	for _, part := range parts {
		switch {
		case strings.EqualFold(part, "ao"):
			name.WriteString("AO")
		case strings.EqualFold(part, "ssbump"):
			name.WriteString("SSBump")
		default:
			runes := []rune(part)
			runes[0] = unicode.ToUpper(runes[0])
			name.WriteString(string(runes))
		}
	}

	result := name.String()
	if result != "" && unicode.IsDigit([]rune(result)[0]) {
		result = "Asset" + result
	}

	if !token.IsIdentifier(result) {
		return ""
	}

	return result
}

// MatchAsset matches a slash-separated asset identity. A ** component spans
// zero or more path components; other components follow path.Match syntax.
func MatchAsset(pattern, identity string) (bool, error) {
	for part := range strings.SplitSeq(pattern, "/") {
		if _, err := path.Match(part, ""); err != nil {
			return false, err
		}
	}

	return matchAssetParts(strings.Split(pattern, "/"), strings.Split(identity, "/"))
}

func matchAssetParts(pattern, identity []string) (bool, error) {
	if len(pattern) == 0 {
		return len(identity) == 0, nil
	}

	if pattern[0] == "**" {
		for index := 0; index <= len(identity); index++ {
			matched, err := matchAssetParts(pattern[1:], identity[index:])
			if err != nil || matched {
				return matched, err
			}
		}

		return false, nil
	}

	if len(identity) == 0 {
		_, err := path.Match(pattern[0], "")

		return false, err
	}

	matched, err := path.Match(pattern[0], identity[0])
	if err != nil || !matched {
		return matched, err
	}

	return matchAssetParts(pattern[1:], identity[1:])
}
