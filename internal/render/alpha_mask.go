package render

import (
	"errors"
	"math"
)

var errAlphaMask = errors.New("invalid Standard alpha-mask material")

// ValidateAlphaMask admits finite Standard cutout thresholds and tint alpha.
// Opaque draws ignore cutoff and alpha, matching their batching contract.
func ValidateAlphaMask(draw DrawCall) error {
	if !draw.AlphaMask {
		return nil
	}

	if draw.Source || draw.Impostor != nil || !alphaMaskUnit(draw.AlphaCutoff) || !alphaMaskUnit(draw.Color[3]) {
		return errAlphaMask
	}

	return nil
}

func alphaMaskUnit(value float32) bool {
	return value >= 0 && value <= 1 && !math.IsNaN(float64(value)) && !math.IsInf(float64(value), 0)
}
