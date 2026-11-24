// Package basis explicitly opts into the legacy ETC1S albedo worker.
// Programs using only offline-prepared native textures should import level
// directly; its default loader does not retain the Basis decoder payload.
package basis

import (
	"github.com/karty-game/ashura/internal/basistranscode"
	"github.com/karty-game/ashura/pkg/engine"
	"github.com/karty-game/ashura/pkg/level"
)

// NewLoader creates an App-owned loader with the previously approved reusable
// ETC1S albedo worker. The decoder is initialized lazily on compressed reads.
// Existing native prepared textures bypass that worker.
func NewLoader(app *engine.App) *level.Loader {
	return level.NewLoaderWithTranscoder(app, basistranscode.New())
}
