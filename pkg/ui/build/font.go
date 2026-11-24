//go:build !js

package build

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"

	"github.com/karty-game/ashura/pkg/assetbuild"
	"github.com/karty-game/ashura/pkg/texture"
	"golang.org/x/image/font/gofont/goregular"
)

const (
	defaultFontPlugin = "ui-default-font"
	fontAtlasSide     = 1024
	fontAtlasBytes    = fontAtlasSide * fontAtlasSide * 4
	fontHeaderBytes   = 92
)

var errDefaultFont = errors.New("invalid prepared Go Regular font")

// DefaultFontOptions selects the offline atlas representation. Zero retains
// exact RGBA8; BC7 retains all three MSDF channels in native GPU blocks.
type DefaultFontOptions struct {
	Format texture.Format
}

type defaultFontPluginOptions struct {
	options []DefaultFontOptions
}

// DefaultFont publishes matching Go Regular atlas and glyphs.bin payloads.
// The embedded WASM library and Go preparation run only during asset builds.
func DefaultFont(options ...DefaultFontOptions) assetbuild.Plugin {
	return defaultFontPluginOptions{options: append([]DefaultFontOptions(nil), options...)}
}

func (defaultFontPluginOptions) Name() string { return defaultFontPlugin }

func (plugin defaultFontPluginOptions) Plan(plan *assetbuild.Plan) error {
	if len(plugin.options) > 1 {
		return errDefaultFont
	}

	format := texture.RGBA8
	if len(plugin.options) == 1 {
		format = plugin.options[0].Format
	}

	if format != texture.RGBA8 && format != texture.BC7 {
		return errDefaultFont
	}

	refs, err := plan.Add(assetbuild.Action{
		ID: defaultFontPlugin, Processor: defaultFontPlugin, Revision: "goregular-msdf@3-bc7",
		Options: struct {
			Source [32]byte       `json:"source"`
			Format texture.Format `json:"format"`
		}{Source: sha256.Sum256(goregular.TTF), Format: format},
		Outputs: []assetbuild.Slot{{Name: filesSlot, Schema: "karty.files@1"}},
		Run: func(ctx *assetbuild.Context) (assetbuild.Result, error) {
			return prepareDefaultFontFormat(ctx, format)
		},
	})
	if err != nil {
		return err
	}

	plan.Publish(refs[0])

	return nil
}

func validateCompressedDefaultFont(metrics, atlas []byte) error {
	if len(atlas) != fontAtlasBytes/4 || len(metrics) < fontHeaderBytes || string(metrics[:8]) != "AFNT0001" {
		return errDefaultFont
	}

	fontHash, atlasHash := sha256.Sum256(goregular.TTF), sha256.Sum256(atlas)
	if string(metrics[28:60]) != string(fontHash[:]) || string(metrics[60:92]) != string(atlasHash[:]) {
		return fmt.Errorf("font/page identity: %w", errDefaultFont)
	}
	// Compression changes only the atlas identity. Keep the qualified glyph
	// roster, distances, dimensions and UVs byte-identical to the raw profile.
	normalized := append([]byte(nil), metrics...)

	rawHash, err := hex.DecodeString("508a4b79048163d70307779e37b6206aa15bd7f27aa9bc49bfcb2afeae070d94")
	if err != nil {
		return err
	}

	copy(normalized[60:92], rawHash)

	metricsHash := sha256.Sum256(normalized)
	if hex.EncodeToString(metricsHash[:]) != "02f987c146170151076dff30d26db4973b4e66c2a3dc3aee112afde5265516a3" {
		return fmt.Errorf("qualified default-font metrics changed: %w", errDefaultFont)
	}

	return nil
}

func validateDefaultFont(metrics, atlas []byte) error {
	if len(atlas) != fontAtlasBytes || len(metrics) < fontHeaderBytes || string(metrics[:8]) != "AFNT0001" {
		return errDefaultFont
	}

	fontHash, atlasHash := sha256.Sum256(goregular.TTF), sha256.Sum256(atlas)
	if string(metrics[28:60]) != string(fontHash[:]) || string(metrics[60:92]) != string(atlasHash[:]) {
		return fmt.Errorf("font/page identity: %w", errDefaultFont)
	}

	// This fixed-source plugin accepts only the complete qualified default pair.
	// Its golden identity also fixes the glyph roster, finite metrics and UV bounds.
	metricsHash := sha256.Sum256(metrics)
	if hex.EncodeToString(atlasHash[:]) != "508a4b79048163d70307779e37b6206aa15bd7f27aa9bc49bfcb2afeae070d94" ||
		hex.EncodeToString(metricsHash[:]) != "02f987c146170151076dff30d26db4973b4e66c2a3dc3aee112afde5265516a3" {
		return fmt.Errorf("qualified default-font output changed: %w", errDefaultFont)
	}

	return nil
}
