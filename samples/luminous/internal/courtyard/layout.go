package courtyard

const (
	HalfWidth      = 14
	HalfDepth      = 11
	BakeResolution = 256
	FloorRed       = .76
	FloorGreen     = .84
	FloorBlue      = .94
)

type Pillar struct {
	X, Z, Width, Height, Depth float32
}

//nolint:mnd // Pillar dimensions and positions are authored scene data.
func Pillars() [5]Pillar {
	return [...]Pillar{
		{X: -5.5, Z: -2.75, Width: 1.25, Height: 4.25, Depth: 1.25},
		{X: 0, Z: -2.75, Width: 1.25, Height: 4.25, Depth: 1.25},
		{X: 5.5, Z: -2.75, Width: 1.25, Height: 4.25, Depth: 1.25},
		{X: -5.5, Z: 4.5, Width: 1.25, Height: 4.25, Depth: 1.25},
		{X: 5.5, Z: 4.5, Width: 1.25, Height: 4.25, Depth: 1.25},
	}
}
