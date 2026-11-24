// Package particle admits prepared particlr effects for Ashura's GPU runtime.
package particle

import (
	"io"

	"github.com/karty-game/ashura/internal/particledata"
)

// Effect contains immutable emitter data and a GPU-ready texture page.
type Effect struct{ prepared *particledata.Effect }

// Read admits a prepared bundle from particle/build. It does not parse .prt.
func Read(reader io.Reader) (*Effect, error) {
	prepared, err := particledata.Read(reader)
	if err != nil {
		return nil, err
	}

	return &Effect{prepared: prepared}, nil
}

// Prepared returns the private data shared with Ashura's engine.
func (effect *Effect) Prepared() *particledata.Effect {
	if effect == nil {
		return nil
	}

	return effect.prepared
}
