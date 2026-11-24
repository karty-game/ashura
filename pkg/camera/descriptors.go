package camera

import "github.com/karty-game/ashura/pkg/engine"

var (
	_ engine.ProjectionProvider = Perspective{}
	_ engine.ProjectionProvider = (*Orthographic)(nil)
)

// ProjectionData supplies named perspective settings to the engine.
func (projection Perspective) ProjectionData() engine.ProjectionData {
	return engine.ProjectionData{
		Kind: engine.PerspectiveProjection, FieldOfViewDegrees: projection.FieldOfViewDegrees,
		Near: projection.Near, Far: projection.Far,
	}
}

// ProjectionData supplies named orthographic settings to the engine. The
// pointer receiver shares ViewHeight with the existing navigation controls.
func (projection *Orthographic) ProjectionData() engine.ProjectionData {
	return engine.ProjectionData{
		Kind: engine.OrthographicProjection, ViewHeight: projection.ViewHeight,
		Near: projection.Near, Far: projection.Far,
	}
}
