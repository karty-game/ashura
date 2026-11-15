package light

import "github.com/karty-game/karty-webgpu/pkg/engine"

var (
	_ engine.LightProvider = Hemisphere{}
	_ engine.LightProvider = Directional{}
	_ engine.LightProvider = Point{}
)

// LightData supplies named hemisphere settings to the engine.
func (source Hemisphere) LightData() engine.LightData {
	return engine.LightData{
		Kind: engine.HemisphereLight, Direction: source.Direction,
		Color: source.SkyColor, GroundColor: source.GroundColor, Intensity: source.Intensity,
	}
}

// LightData supplies named directional settings to the engine.
func (source Directional) LightData() engine.LightData {
	return engine.LightData{Kind: engine.DirectionalLight, Direction: source.Direction, Color: source.Color, Intensity: source.Intensity}
}

// LightData supplies named point-light settings without packing range or shadow
// metadata into a color field. Position is independent of the entity Transform.
func (source Point) LightData() engine.LightData {
	return engine.LightData{
		Kind: engine.PointLight, Position: source.Position, Color: source.Color,
		Intensity: source.Intensity, Range: source.Range,
		Shadow: engine.PointShadow{
			Enabled: source.CastShadows, Resolution: source.ShadowResolution,
			Bias: source.ShadowBias, DebugFaces: source.DebugFaces,
		},
	}
}
