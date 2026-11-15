package material

// DefaultStandard returns an opaque white Standard material with unit UV scale
// and normal/AO strength. Set fields directly before World.Add; explicit zeros
// remain zero because no options normalization runs after this function returns.
// Roughness is zero, matching Standard's existing smooth material default.
func DefaultStandard() Material {
	return Standard(Options{})
}

// DefaultSource returns the existing neutral RNM fill and unit SSBump strength.
// Set fields directly before World.Add; zero AmbientColors selects no ambient
// fill and zero SSBumpStrength disables the map without being replaced by defaults.
func DefaultSource() SourceMaterial {
	return Source(SourceOptions{})
}
