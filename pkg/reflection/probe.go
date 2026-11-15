// Package reflection provides placed, box-projected cubemap probes.
package reflection

import (
	"github.com/karty-game/karty-webgpu/pkg/engine"
	"github.com/karty-game/karty-webgpu/pkg/texture"
)

// ProbeOptions configures a probe's axis-aligned influence volume. HalfExtents
// are measured from the probe entity's Transform.Position in world units.
type ProbeOptions struct {
	HalfExtents engine.Vector3
	Strength    float32
}

// Probe binds a prepared six-face cubemap to a placed influence volume.
// Reflective Standard materials inside the volume select this probe
// automatically; smaller overlapping volumes take priority.
type Probe struct {
	Name        string
	Environment *texture.Texture
	HalfExtents engine.Vector3
	Strength    float32
}

// DefaultProbe returns a probe at unit strength. Set HalfExtents and other
// fields before World.Add. Setting Strength to zero disables this probe;
// unlike NewProbe's compatibility options, no later default replaces it.
func DefaultProbe(name string, environment *texture.Texture) Probe {
	return NewProbe(name, environment, ProbeOptions{})
}

// NewProbe declares a cubemap probe. Attach the returned component and an
// engine.Transform to the same world entity. The environment is normally loaded
// from a build-generated ktx2.Reflection asset.
// Zero Strength retains its compatibility meaning of one; use DefaultProbe
// followed by direct field assignment when zero must disable the probe.
func NewProbe(name string, environment *texture.Texture, options ProbeOptions) Probe {
	strength := options.Strength
	if strength == 0 {
		strength = 1
	}

	return Probe{Name: name, Environment: environment, HalfExtents: options.HalfExtents, Strength: strength}
}

// RuntimeReflectionProbe exposes the probe's immutable environment and volume.
func (probe Probe) RuntimeReflectionProbe() (*texture.Texture, engine.Vector3, float32) {
	return probe.Environment, probe.HalfExtents, probe.Strength
}
