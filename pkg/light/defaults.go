package light

// DefaultHemisphere returns the default sky/ground fill at unit intensity.
// Set component fields directly before World.Add, including zero Intensity to
// start disabled. No options normalization runs after this function returns.
func DefaultHemisphere(name string) Hemisphere {
	return NewHemisphere(name, HemisphereOptions{})
}

// DefaultDirectional returns a white light pointing down at unit intensity.
// Set fields directly before World.Add; explicit zero Intensity remains zero.
func DefaultDirectional(name string) Directional {
	return NewDirectional(name, DirectionalOptions{})
}

// DefaultPoint returns a white light with unit intensity, range 20 and shadows
// disabled. Its optional shadow settings are resolution 512 and bias 0.015.
// Set fields directly before World.Add; explicit zero Intensity or ShadowBias
// remains zero instead of selecting a constructor default.
func DefaultPoint(name string) Point {
	return NewPoint(name, PointOptions{})
}
