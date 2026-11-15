package lightmapper

import "math"

const lightPi = float32(math.Pi)

func Add(a, b Vec3) Vec3           { return Vec3{a[0] + b[0], a[1] + b[1], a[2] + b[2]} }
func Sub(a, b Vec3) Vec3           { return Vec3{a[0] - b[0], a[1] - b[1], a[2] - b[2]} }
func Scale(a Vec3, s float32) Vec3 { return Vec3{a[0] * s, a[1] * s, a[2] * s} }
func Mul(a, b Vec3) Vec3           { return Vec3{a[0] * b[0], a[1] * b[1], a[2] * b[2]} }
func Dot(a, b Vec3) float32        { return a[0]*b[0] + a[1]*b[1] + a[2]*b[2] }
func Cross(a, b Vec3) Vec3 {
	return Vec3{a[1]*b[2] - a[2]*b[1], a[2]*b[0] - a[0]*b[2], a[0]*b[1] - a[1]*b[0]}
}
func Length(vector Vec3) float32 {
	maximum := max(float32(math.Abs(float64(vector[0]))), float32(math.Abs(float64(vector[1]))), float32(math.Abs(float64(vector[2]))))
	if maximum == 0 {
		return 0
	}

	scaled := Vec3{vector[0] / maximum, vector[1] / maximum, vector[2] / maximum}

	return maximum * float32(math.Sqrt(float64(Dot(scaled, scaled))))
}
func Normalize(vector Vec3) Vec3 {
	length := Length(vector)
	if length == 0 {
		return Vec3{}
	}

	return Vec3{vector[0] / length, vector[1] / length, vector[2] / length}
}
func luminance(a Vec3) float32    { return a[0]*.2126 + a[1]*.7152 + a[2]*.0722 }
func finite(v float32) bool       { return !math.IsNaN(float64(v)) && !math.IsInf(float64(v), 0) }
func validVec(v Vec3) bool        { return finite(v[0]) && finite(v[1]) && finite(v[2]) }
func positiveVec(v Vec3) bool     { return validVec(v) && v[0] >= 0 && v[1] >= 0 && v[2] >= 0 }
func maxComponent(v Vec3) float32 { return max(v[0], v[1], v[2]) }

//nolint:mnd // The pole threshold selects a stable auxiliary axis for the orthonormal frame.
func tangentFrame(normal Vec3) (Vec3, Vec3) {
	axis := Vec3{0, 1, 0}
	if float32(math.Abs(float64(normal[1]))) > .9 {
		axis = Vec3{1, 0, 0}
	}

	tangent := Normalize(Cross(axis, normal))

	return tangent, Cross(normal, tangent)
}

//nolint:mnd // The azimuth spans the complete 2pi circle.
func sampleHemisphere(normal Vec3, polarSample, azimuthSample float32, cosine bool) Vec3 {
	axial := polarSample
	if cosine {
		axial = float32(math.Sqrt(float64(polarSample)))
	}

	radial := float32(math.Sqrt(float64(max(0, 1-axial*axial))))
	angle := 2 * lightPi * azimuthSample
	tangent, bitangent := tangentFrame(normal)

	return Add(
		Scale(normal, axial),
		Add(Scale(tangent, radial*float32(math.Cos(float64(angle)))), Scale(bitangent, radial*float32(math.Sin(float64(angle))))),
	)
}
func mixVec(a, b Vec3, t float32) Vec3 { return Add(Scale(a, 1-t), Scale(b, t)) }
func minVec(a, b Vec3) Vec3            { return Vec3{min(a[0], b[0]), min(a[1], b[1]), min(a[2], b[2])} }
func maxVec(a, b Vec3) Vec3            { return Vec3{max(a[0], b[0]), max(a[1], b[1]), max(a[2], b[2])} }
