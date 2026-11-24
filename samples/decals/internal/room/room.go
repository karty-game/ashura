// Package room shares the yard's scene geometry and paint grid with the asset recipe.
package room

import (
	"github.com/karty-game/ashura/pkg/collision"
	"github.com/karty-game/ashura/pkg/engine"
	"github.com/karty-game/ashura/pkg/mesh"
)

const (
	WallLayer uint32 = 1 << iota
	FloorLayer
	CarLayer
)

const (
	WallFront = -4.15
	CarFront  = 1.15
)

type Finish uint8

const (
	Concrete Finish = iota
	Charcoal
	Steel
	Rust
	Teal
	Amber
	Lamp
)

type Surface struct {
	Bounds collision.Bounds
	Layers uint32
	Finish Finish
}

// Panel is a front-facing paintable area. Grid dimensions are shared at build
// time so the editor can address prepared projectors without creating them.
type Panel struct {
	Name                               string
	Left, Bottom, Width, Height, Front float32
	Cols, Rows                         int
	Layers                             uint32
}

func Panels() [3]Panel {
	return [3]Panel{
		{"Mural", -6.6, .42, 12.35, 4.1, WallFront, 54, 17, WallLayer},
		{"Service", -12.5, .34, 5.1, 3.95, WallFront, 21, 16, WallLayer},
		{"Freight", 5.95, .99, 5.35, 2.56, CarFront, 22, 11, CarLayer},
	}
}

func (panel Panel) Count() int { return panel.Cols * panel.Rows }

func (panel Panel) Spacing() (float32, float32) {
	return panel.Width / float32(panel.Cols), panel.Height / float32(panel.Rows)
}

func (panel Panel) Center(index int) engine.Vector3 {
	const half = .5

	dx, dy := panel.Spacing()

	return engine.Vector3{
		X: panel.Left + (float32(index%panel.Cols)+half)*dx,
		Y: panel.Bottom + (float32(index/panel.Cols)+half)*dy,
		Z: panel.Front,
	}
}

func (panel Panel) Cell(point, normal engine.Vector3) (int, bool) {
	if normal.Z < .98 || point.Z < panel.Front-.015 || point.Z > panel.Front+.015 ||
		point.X < panel.Left || point.X >= panel.Left+panel.Width || point.Y < panel.Bottom || point.Y >= panel.Bottom+panel.Height {
		return 0, false
	}

	dx, dy := panel.Spacing()
	column := min(panel.Cols-1, int((point.X-panel.Left)/dx))
	row := min(panel.Rows-1, int((point.Y-panel.Bottom)/dy))

	return row*panel.Cols + column, true
}

func Surfaces() []Surface {
	result := []Surface{
		part(-13, 0, -4.74, -6.95, 5.2, WallFront, WallLayer, Concrete),
		part(-6.9, 0, -4.74, -.1, 5.2, WallFront, WallLayer, Concrete),
		part(-.09, 0, -4.74, 6.12, 5.2, WallFront, WallLayer, Concrete),
		part(6.15, 0, -4.74, 13, 5.2, WallFront, WallLayer, Concrete),
		part(-13.2, 0, -7.2, -12.95, 5.4, -2.7, 0, Charcoal),
		part(11.55, 0, -7, 13.05, 6.6, -4.9, 0, Charcoal),
		part(-11.8, 0, -7.35, -8.1, 6.54, -5.85, 0, Charcoal),
		part(-11.95, 6.48, -7.5, -7.95, 6.69, -5.74, 0, Steel),
		part(-10.96, 6.68, -7.10, -10.55, 8.24, -6.58, 0, Rust),
		part(-11.04, 8.20, -7.20, -10.47, 8.39, -6.48, 0, Charcoal),
		part(5.7, 0, -8.0, 10.91, 7.34, -6.28, 0, Charcoal),
		part(5.52, 7.26, -8.12, 11.11, 7.46, -6.13, 0, Steel),
		part(6.34, 6.04, -6.27, 10.27, 6.73, -6.18, 0, Steel),
		part(-3.07, 5.8, -6.63, 5.75, 6.13, -6.39, 0, Charcoal),
		part(-12.45, 5.14, -4.98, 12.45, 5.38, -3.88, 0, Steel),
		part(-12.6, .04, -4.24, 12.6, .24, -3.95, 0, Charcoal),
		part(-6.95, 4.65, -4.23, 6.2, 4.88, -4.01, 0, Steel),
		part(-12.3, 4.46, -4.25, -7.3, 4.61, -3.98, 0, Steel),
		part(5.32, .40, -.29, 11.96, .76, 1.39, 0, Charcoal),
		part(5.52, .77, -1.01, 11.85, 3.68, CarFront, CarLayer, Teal),
		part(5.44, .74, -1.1, 11.94, .92, 1.26, 0, Steel),
		part(5.44, 3.56, -1.1, 11.94, 3.74, 1.26, 0, Steel),
		part(5.35, .83, -1.08, 5.83, 3.69, 1.28, 0, Teal),
		part(11.42, .83, -1.08, 11.94, 3.69, 1.28, 0, Teal),
		part(-11.9, .03, -.3, -9.35, 1.22, 1.32, 0, Rust),
		part(-11.6, 1.22, -.25, -10.0, 2.1, .99, 0, Rust),
		part(-8.94, .04, .18, -8.32, .82, .8, 0, Amber),
		part(-8.8, .82, .32, -8.46, 1.14, .66, 0, Charcoal),
		part(2, .03, 3.72, 2.22, .64, 3.94, 0, Amber),
		part(3.6, .03, 3.72, 3.82, .64, 3.94, 0, Amber),
		part(2.08, .45, 3.77, 3.75, .56, 3.9, 0, Amber),
	}
	for _, x := range []float32{-12.85, -6.92, 6.13, 12.75} {
		result = append(result,
			part(x-.12, .22, -4.27, x+.12, 5.2, -3.91, 0, Steel),
			part(x-.24, .02, -4.32, x+.24, .32, -3.75, 0, Charcoal),
		)
	}

	for _, x := range []float32{-9.65, 2.2, 9.8} {
		result = append(result,
			part(x-.055, 4.75, -4.32, x+.055, 5.54, -4.01, 0, Charcoal),
			part(x-.31, 4.72, -4.03, x+.31, 4.88, -3.62, 0, Steel),
			part(x-.24, 4.70, -4.00, x+.24, 4.73, -3.67, 0, Lamp),
		)
	}

	for _, x := range []float32{6.16, 6.94, 10.12, 10.90} {
		result = append(result, part(x-.27, .075, -.08, x+.27, .62, .92, 0, Charcoal))
	}

	for x := float32(-13); x < 13; x += .85 {
		result = append(result, part(x, .012, 4.66, x+.20, .072, 6.22, 0, Rust))
	}

	for _, z := range []float32{4.9, 5.98} {
		result = append(result, part(-13.65, .065, z, 13.65, .13, z+.075, 0, Steel))
	}

	return result
}

func GroundBounds() collision.Bounds {
	return part(-14.4, -.19, -7.6, 14.4, 0, 9.65, 0, Concrete).Bounds
}

// Floor returns a floor quad with a single albedo chart for permanent marks.
func Floor() (mesh.Mesh, engine.Transform) {
	const half = .5

	bounds := GroundBounds()
	lo, hi := bounds.Min, bounds.Max
	transform := engine.DefaultTransform()
	transform.Position = engine.Vector3{X: (lo.X + hi.X) * half, Y: hi.Y, Z: (lo.Z + hi.Z) * half}

	return mesh.NewPlane("switchyard asphalt", mesh.PlaneOptions{Width: hi.X - lo.X, Height: hi.Z - lo.Z}), transform
}

func part(left, bottom, back, right, top, front float32, layers uint32, finish Finish) Surface {
	return Surface{
		Bounds: collision.Bounds{Min: engine.Vector3{X: left, Y: bottom, Z: back}, Max: engine.Vector3{X: right, Y: top, Z: front}},
		Layers: layers, Finish: finish,
	}
}

type Paint struct {
	Name  string
	Color engine.Color
}

//nolint:mnd // The palette uses authored linear paint colors.
func Paints() [3]Paint {
	return [3]Paint{
		{"Cyan", engine.Color{R: .035, G: .97, B: .90, A: .9}},
		{"Pink", engine.Color{R: 1, G: .036, B: .36, A: .9}},
		{"Gold", engine.Color{R: 1, G: .57, B: .026, A: .9}},
	}
}
