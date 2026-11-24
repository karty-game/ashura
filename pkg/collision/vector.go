package collision

import (
	"math"

	"github.com/karty-game/ashura/pkg/engine"
)

type vector [3]float64

func fromEngine(v engine.Vector3) vector {
	return vector{float64(v.X), float64(v.Y), float64(v.Z)}
}

func (a vector) engine() engine.Vector3 {
	return engine.Vector3{X: float32(a[0]), Y: float32(a[1]), Z: float32(a[2])}
}

func (a vector) add(b vector) vector  { return vector{a[0] + b[0], a[1] + b[1], a[2] + b[2]} }
func (a vector) sub(b vector) vector  { return vector{a[0] - b[0], a[1] - b[1], a[2] - b[2]} }
func (a vector) mul(t float64) vector { return vector{a[0] * t, a[1] * t, a[2] * t} }
func (a vector) dot(b vector) float64 { return a[0]*b[0] + a[1]*b[1] + a[2]*b[2] }

func (a vector) cross(b vector) vector {
	return vector{a[1]*b[2] - a[2]*b[1], a[2]*b[0] - a[0]*b[2], a[0]*b[1] - a[1]*b[0]}
}

func (a vector) finite() bool        { return finite(a[0]) && finite(a[1]) && finite(a[2]) }
func finite(t float64) bool          { return !math.IsNaN(t) && !math.IsInf(t, 0) }
func unitInterval(t float64) float64 { return max(0, min(1, t)) }
