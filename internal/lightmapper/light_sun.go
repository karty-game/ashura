//nolint:mnd // Angular-capArea formulas retain their explicit trigonometric constants.
package lightmapper

import "math"

// sunCap avoids subtracting two almost equal float32 values for narrow suns.
func sunCap(radius float32) float32 {
	s := float32(math.Sin(float64(radius) * .5))

	return 2 * s * s
}
func sampleSun(emitter Emitter, index uint32, radialSample, angularSample float32) LightSample {
	axis := Scale(emitter.Direction, -1)
	capArea := sunCap(emitter.Radius)
	loss := radialSample * capArea
	axial := 1 - loss
	radial := float32(math.Sqrt(float64(max(0, loss*(2-loss)))))
	angle := 2 * lightPi * angularSample
	tangent, bitangent := tangentFrame(axis)

	return LightSample{
		Emitter: index,
		Direction: Add(
			Scale(axis, axial),
			Add(Scale(tangent, radial*float32(math.Cos(float64(angle)))), Scale(bitangent, radial*float32(math.Sin(float64(angle))))),
		),
		Radiance: emitter.Intensity,
		Distance: math.MaxFloat32,
		PDF:      1 / (2 * lightPi * capArea),
	}
}
func sunPDF(emitter Emitter, direction Vec3) float32 {
	axis := Scale(emitter.Direction, -1)

	angle := math.Atan2(float64(Length(Cross(axis, direction))), float64(Dot(axis, direction)))
	if angle > float64(emitter.Radius) {
		return 0
	}

	return 1 / (2 * lightPi * sunCap(emitter.Radius))
}
