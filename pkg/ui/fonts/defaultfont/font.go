// Package defaultfont supplies the optional, offline-prepared Go Regular MSDF face.
// Its prepared payloads are reproducible with the tracked asset pipeline recipe.
package defaultfont

import (
	_ "embed"

	"github.com/karty-game/ashura/pkg/ui"
	"golang.org/x/image/font/gofont/goregular"
)

// Regeneration uses asset.config.go and the embedded WASM library through the
// normal assets CLI. See README.md for the command and provenance.

//go:embed prepared/atlas.rgba
var atlas []byte

//go:embed prepared/glyphs.bin
var metrics []byte

// Load copies/adopts the prepared page and admits the Go Regular shaping tables.
func Load() (*ui.Font, error) { return ui.LoadFont(goregular.TTF, metrics, atlas) }
