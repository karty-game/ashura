// Package decal defines projected surface markings for Standard and Source meshes.
package decal

import "github.com/karty-game/ashura/pkg/engine"

// MaxProjectors is the number of projector components allowed in one world.
const MaxProjectors = engine.MaxDecalProjectors

type Image = engine.DecalImage
type Projector = engine.DecalProjector
type Receiver = engine.DecalReceiver
type Prepared = engine.PreparedDecals
type Placement = engine.DecalPlacement

// DecodePrepared reads the offline projector records for one authored batch.
func DecodePrepared(data []byte) (*Prepared, error) { return engine.DecodePreparedDecals(data) }

// DefaultProjector returns a white, downward-facing projector on receiver layer one.
func DefaultProjector(image Image) Projector {
	const defaultDepth = .1

	return Projector{
		Image: image, Normal: engine.Vector3{Y: 1},
		Size: engine.Vector3{X: 1, Y: 1, Z: defaultDepth},
		Tint: engine.ColorWhite(), ReceiverMask: 1,
	}
}
