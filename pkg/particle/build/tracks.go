//go:build !js

package build

import "fmt"

func validateTrack(property *track, lowerBound float64) error {
	if property == nil {
		return nil
	}

	switch property.Mode {
	case constantTrack:
		if between(property.Value, lowerBound, 1e6) {
			return nil
		}
	case "curve":
		if len(property.Keys) == 0 || len(property.Keys) > 64 {
			return errDocument
		}

		last := -1.0
		for _, key := range property.Keys {
			if !between(key.T, 0, 1) || key.T <= last || !between(key.V, lowerBound, 1e6) {
				return errDocument
			}

			switch key.Ease {
			case "", "linear", "easeIn", "easeOut", "easeInOut":
			default:
				return fmt.Errorf("curve easing %q: %w", key.Ease, errUnsupported)
			}

			last = key.T
		}

		return nil
	default:
		return fmt.Errorf("lifetime track %q: %w", property.Mode, errUnsupported)
	}

	return errDocument
}

func sampledTrack(property *track, age, fallback float64) float64 {
	if property == nil {
		return fallback
	}

	if property.Mode == constantTrack {
		return property.Value
	}

	keys := property.Keys
	for index, right := range keys {
		if age > right.T {
			continue
		}

		if index == 0 {
			return right.V
		}

		left := keys[index-1]

		fraction := (age - left.T) / (right.T - left.T)
		switch left.Ease {
		case "easeIn":
			fraction *= fraction
		case "easeOut":
			fraction = 1 - (1-fraction)*(1-fraction)
		case "easeInOut":
			const (
				midpoint = .5
				slope    = 2
			)
			if fraction < midpoint {
				fraction = slope * fraction * fraction
			} else {
				fraction = 1 - 2*(1-fraction)*(1-fraction)
			}
		}

		return lerp(left.V, right.V, fraction)
	}

	return keys[len(keys)-1].V
}

func validateGradient(keys []gradientKey) error {
	if len(keys) == 0 || len(keys) > 64 {
		return errDocument
	}

	last := -1.0
	for _, key := range keys {
		if !between(key.T, 0, 1) || key.T <= last || !between(key.R, 0, 1) ||
			!between(key.G, 0, 1) || !between(key.B, 0, 1) || !between(key.A, 0, 1) {
			return errDocument
		}

		last = key.T
	}

	return nil
}

func gradient(keys []gradientKey, age float64) [4]float64 {
	for index, right := range keys {
		if age > right.T {
			continue
		}

		if index == 0 {
			return [4]float64{right.R, right.G, right.B, right.A}
		}

		left := keys[index-1]
		fraction := (age - left.T) / (right.T - left.T)

		return [4]float64{
			lerp(left.R, right.R, fraction), lerp(left.G, right.G, fraction),
			lerp(left.B, right.B, fraction), lerp(left.A, right.A, fraction),
		}
	}

	last := keys[len(keys)-1]

	return [4]float64{last.R, last.G, last.B, last.A}
}

func lerp(a, b, t float64) float64 { return a + (b-a)*t }
