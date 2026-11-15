package impostor

import (
	"errors"
	"math"

	"github.com/karty-game/karty-webgpu/internal/impostordata"
)

const (
	defaultFarPixels   = 16
	defaultNearPixels  = 128
	maximumCorrections = 2
	maximumMipBias     = 4
	maximumNearPixels  = 32768
	qualityRampScale   = 2
)

var errQuality = errors.New(
	"impostor quality requires 0..2 corrections, finite mip bias in [-4,4], and thresholds 0 < far*2 <= near <= 32768 pixels",
)

// Quality controls octahedral reprojection, independently of baked mip resolution.
// Settings are shared by every instance of this loaded asset. Other capture
// modes retain their own rendering path and ignore these settings.
type Quality struct {
	// Adaptive selects corrections from the projected sphere diameter.
	Adaptive bool
	// FarPixels starts the smooth zero-to-one correction ramp, ending at twice
	// this diameter. NearPixels likewise starts the one-to-two correction ramp.
	FarPixels, NearPixels float32
	// MaxCorrections caps adaptive work, or fixes the correction count when
	// Adaptive is false. Zero, one and two use six, nine and twelve texture reads.
	MaxCorrections int
	// MipBias offsets derivative-based image mip selection. Positive is coarser.
	MipBias float32
}

// ECS components are copied by value; this one shared state keeps every
// instance attached to edits on the loaded resource without per-instance writes.
type qualityState struct {
	settings   Quality
	parameters [4]float32
}

// DefaultQuality uses zero corrections through 16px, one by 32px, and two by
// 256px, with the second ramp starting at 128px and no mip bias.
func DefaultQuality() Quality {
	return Quality{Adaptive: true, FarPixels: defaultFarPixels, NearPixels: defaultNearPixels, MaxCorrections: maximumCorrections}
}

// Validate checks shader work and numeric bounds without changing a resource.
func (settings Quality) Validate() error {
	_, err := qualityParameters(settings)

	return err
}

// SetQuality changes bounded shader parameters on the owning App goroutine.
// It does not bake, repack or upload asset data; ordinary material dirtiness
// updates the existing uniform when a shader-visible setting changes.
func (value *Impostor) SetQuality(settings Quality) error {
	if value == nil || !value.loaded || value.app.Closed() {
		return errUnavailable
	}

	encoded, err := qualityParameters(settings)
	if err != nil {
		return err
	}

	value.quality.settings, value.quality.parameters = settings, encoded

	return nil
}

// SupportsQuality reports whether this resource uses octahedral reprojection.
func (value *Impostor) SupportsQuality() bool {
	return value != nil && value.document != nil && value.document.Mode == impostordata.ModeOctahedral
}

// Quality returns the loaded asset's current correction and filtering settings.
func (value *Impostor) Quality() Quality { return value.quality.settings }

// RuntimeImpostorQuality supplies four cached scalar values, without packing
// the immutable view table or doing camera-dependent CPU work.
func (value *Impostor) RuntimeImpostorQuality() [4]float32 { return value.quality.parameters }

func qualityParameters(settings Quality) ([4]float32, error) {
	if settings.MaxCorrections < 0 || settings.MaxCorrections > maximumCorrections || !finiteQuality(settings.MipBias) ||
		settings.MipBias < -maximumMipBias || settings.MipBias > maximumMipBias {
		return [4]float32{}, errQuality
	}

	if !settings.Adaptive {
		return [4]float32{0, 0, -float32(settings.MaxCorrections) - 1, settings.MipBias}, nil
	}

	if !finiteQuality(settings.FarPixels) || !finiteQuality(settings.NearPixels) || settings.FarPixels <= 0 ||
		settings.NearPixels < settings.FarPixels*qualityRampScale || settings.NearPixels > maximumNearPixels {
		return [4]float32{}, errQuality
	}

	return [4]float32{settings.FarPixels, settings.NearPixels, float32(settings.MaxCorrections), settings.MipBias}, nil
}

func finiteQuality(value float32) bool {
	return !math.IsNaN(float64(value)) && !math.IsInf(float64(value), 0)
}
