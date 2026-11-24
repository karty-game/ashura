//go:build !js

// Package build prepares particlr documents for Ashura.
package build

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"math"
	"slices"

	"github.com/karty-game/ashura/internal/particledata"
)

const (
	SchemaVersion = 13
	constantTrack = "constant"
	coneShape     = "cone"
	layerSeedStep = 0x9e3779b9
)

var (
	errDocument    = errors.New("invalid particlr document")
	errUnsupported = errors.New("unsupported particlr feature")
)

// Compile translates a version 13 .prt document into a prepared particle bundle.
// The limited feature set is reported as a build error when a document uses
// unsupported behavior. Compile leaves destination untouched on admission error.
func Compile(destination io.Writer, source io.Reader) error {
	return Compile3D(destination, source, nil)
}

// Compile3D applies offline spatial settings to enabled layers by their .prt ID.
// Layers without settings retain their authored planar motion.
func Compile3D(destination io.Writer, source io.Reader, spatial SpatialLayers) error {
	input, err := io.ReadAll(io.LimitReader(source, particledata.MaxBundleBytes+1))
	if err != nil {
		return err
	}

	if len(input) > particledata.MaxBundleBytes {
		return errDocument
	}

	var doc document

	decoder := json.NewDecoder(bytes.NewReader(input))
	decoder.DisallowUnknownFields()

	if err := decoder.Decode(&doc); err != nil {
		return fmt.Errorf("particlr: %w", err)
	}

	var tail json.RawMessage
	if err := decoder.Decode(&tail); err != io.EOF {
		return errDocument
	}

	effect, err := prepare(doc, spatial)
	if err != nil {
		return err
	}

	var bundle bytes.Buffer
	if err := particledata.Write(&bundle, effect); err != nil {
		return err
	}

	if bundle.Len() > particledata.MaxBundleBytes {
		return errDocument
	}

	_, err = destination.Write(bundle.Bytes())

	return err
}

func prepare(doc document, spatial SpatialLayers) (*particledata.Effect, error) {
	if doc.SchemaVersion != SchemaVersion {
		return nil, fmt.Errorf("schema version %d: %w", doc.SchemaVersion, errUnsupported)
	}

	if !between(doc.Duration, .05, 3600) || len(doc.Layers) == 0 || len(doc.Layers) > particledata.MaxLayers {
		return nil, errDocument
	}

	if len(doc.Params) != 0 {
		return nil, fmt.Errorf("host parameters: %w", errUnsupported)
	}

	page, regions, err := preparePage(doc)
	if err != nil {
		return nil, err
	}

	effect := &particledata.Effect{Duration: doc.Duration, Loop: doc.Looping, Page: page}
	remaining := maps.Clone(spatial)

	for index, source := range doc.Layers {
		if !source.Enabled {
			continue
		}

		seed := doc.Seed + uint32(index)*layerSeedStep

		result, err := prepareLayer(source, doc.Duration, seed, regions[source.Texture.Ref])
		if err != nil {
			return nil, fmt.Errorf("particlr layer %q: %w", source.ID, err)
		}

		if settings, ok := spatial[source.ID]; ok {
			if _, available := remaining[source.ID]; !available {
				return nil, fmt.Errorf("duplicate spatial layer ID %q: %w", source.ID, errDocument)
			}

			if err := packSpatial(result.Config, source.Shape.Kind, settings); err != nil {
				return nil, fmt.Errorf("particlr layer %q: %w", source.ID, err)
			}

			delete(remaining, source.ID)
		}

		if source.Emission.Prewarm {
			if err := bakeWarm(&result, doc.Duration); err != nil {
				return nil, fmt.Errorf("particlr layer %q: %w", source.ID, err)
			}
		}

		effect.Layers = append(effect.Layers, result)
	}

	if len(remaining) != 0 {
		return nil, fmt.Errorf("unknown or disabled spatial layers %q: %w", slices.Sorted(maps.Keys(remaining)), errDocument)
	}

	if len(effect.Layers) == 0 {
		return nil, fmt.Errorf("no enabled layers: %w", errDocument)
	}

	return effect, nil
}

func prepareLayer(source layer, duration float64, seed uint32, region spriteRegion) (particledata.Layer, error) {
	var result particledata.Layer
	if err := supported(source); err != nil {
		return result, err
	}

	if region.aspect == 0 {
		return result, fmt.Errorf("texture %q: %w", source.Texture.Ref, errUnsupported)
	}

	switch source.Blend {
	case "normal":
		result.Blend = particledata.Normal
	case "add":
		result.Blend = particledata.Add
	default:
		return result, fmt.Errorf("blend %q: %w", source.Blend, errUnsupported)
	}

	emission := source.Emission
	if emission.RateOverTime.Mode != constantTrack {
		return result, fmt.Errorf("emission rate track %q: %w", emission.RateOverTime.Mode, errUnsupported)
	}

	if emission.MaxParticles == 0 || emission.MaxParticles > particledata.MaxParticles ||
		!between(emission.RateOverTime.Value, 0, particledata.MaxParticles) ||
		!between(emission.Delay, 0, duration) || len(emission.Bursts) > particledata.MaxBursts {
		return result, errDocument
	}

	result.Capacity, result.Rate, result.Delay = emission.MaxParticles, emission.RateOverTime.Value, emission.Delay
	for _, entry := range emission.Bursts {
		if entry.Spread != 0 || entry.Cycles != 1 || entry.Probability != 1 {
			return result, fmt.Errorf("spread, repeat or probabilistic burst: %w", errUnsupported)
		}

		time := emission.Delay + entry.Time
		if !between(time, emission.Delay, duration) || entry.Count == 0 || entry.Count > particledata.MaxParticles {
			return result, errDocument
		}

		result.Bursts = append(result.Bursts, particledata.Burst{Time: time, Count: entry.Count})
	}

	config, life, err := packLayer(source, seed, region)
	if err != nil {
		return result, err
	}

	result.Config, result.MaxLife = config, life

	return result, nil
}

func supported(source layer) error {
	switch {
	case source.Space != "local":
		return fmt.Errorf("simulation space %q: %w", source.Space, errUnsupported)
	case source.InheritVelocity != 0 || source.AttractorInfluence != 0:
		return fmt.Errorf("host velocity or attractor: %w", errUnsupported)
	case source.Emission.RateOverTimeParam != nil || source.Emission.RateOverDistanceParam != nil ||
		source.Initial.LifeParam != nil || source.Initial.SpeedParam != nil || source.Initial.SizeParam != nil ||
		source.OverLifetime.Velocity.GravityParam != nil || source.OpacityParam != nil || source.TintParam != nil:
		return fmt.Errorf("parameter binding: %w", errUnsupported)
	}

	fields := []struct {
		name  string
		value json.RawMessage
	}{
		{"flipbook", source.Texture.Frames}, {"rate over distance", source.Emission.RateOverDistance},
		{"velocity multiplier", source.OverLifetime.Velocity.SpeedMultiplier},
		{"velocity x", source.OverLifetime.Velocity.X}, {"velocity y", source.OverLifetime.Velocity.Y},
		{"orbital velocity", source.OverLifetime.Velocity.Orbital}, {"radial velocity", source.OverLifetime.Velocity.Radial},
		{"velocity limit", source.LimitVelocity}, {"wind", source.Wind}, {"by-speed tracks", source.BySpeed},
		{"by-emitter-speed tracks", source.ByEmitterSpeed}, {"start color", source.StartColor},
		{"random flip", source.RandomFlip}, {"dissolve", source.Dissolve},
		{"collision", source.Collision}, {"kill zones", source.KillZones}, {"attractor", source.Attractor},
		{"sub-emitters", source.SubEmitters}, {"trail", source.Trail},
	}
	for _, field := range fields {
		if len(field.value) != 0 && string(field.value) != "null" {
			return fmt.Errorf("%s: %w", field.name, errUnsupported)
		}
	}

	return nil
}

func packLayer(source layer, seed uint32, region spriteRegion) ([]byte, float64, error) {
	config := make([]byte, particledata.ConfigBytes)
	put := func(index int, value float64) {
		binary.LittleEndian.PutUint32(config[index*4:], math.Float32bits(float32(value)))
	}

	if err := packInitial(source, put); err != nil {
		return nil, 0, err
	}

	gravity := source.OverLifetime.Velocity.Gravity
	if !between(gravity.X, -1e6, 1e6) || !between(gravity.Y, -1e6, 1e6) {
		return nil, 0, errDocument
	}

	put(10, gravity.X)
	put(11, gravity.Y)

	shape, surface, err := shapeValues(source.Shape)
	if err != nil {
		return nil, 0, err
	}

	for index, value := range shape {
		put(12+index, value)
	}

	var strength *track

	if noise := source.Noise; noise != nil {
		if noise.Octaves < 1 || noise.Octaves > 3 || !between(noise.Frequency, 0, 1) ||
			!between(noise.ScrollSpeed, -100, 100) {
			return nil, 0, errDocument
		}

		strength = &noise.Strength
		put(17, noise.Frequency)
		put(18, noise.ScrollSpeed)
		put(19, float64(noise.Octaves))
	}

	for index, value := range region.uv {
		put(20+index, value)
	}

	binary.LittleEndian.PutUint32(config[24*4:], seed)
	put(25, surface)
	put(26, region.aspect)

	if err := packTracks(source, strength, put); err != nil {
		return nil, 0, err
	}

	if err := packRender(config, source.Render); err != nil {
		return nil, 0, err
	}

	life := float64(math.Float32frombits(binary.LittleEndian.Uint32(config[4:])))

	return config, life, nil
}

func packInitial(source layer, put func(int, float64)) error {
	ranges := []track{
		source.Initial.Life,
		source.Initial.Speed,
		source.Initial.Size,
		source.Initial.Rotation,
		source.Initial.AngularVelocity,
	}
	for index, property := range ranges {
		low, high := property.Value, property.Value
		switch property.Mode {
		case constantTrack:
		case "range":
			low, high = property.Min, property.Max
		default:
			return fmt.Errorf("initial track %q: %w", property.Mode, errUnsupported)
		}

		if !between(low, -1e6, high) || high > 1e6 || (index == 0 && !between(low, .001, 3600)) ||
			(index == 0 && high > 3600) || ((index == 1 || index == 2) && low < 0) {
			return errDocument
		}

		put(index*2, low)
		put(index*2+1, high)
	}

	return nil
}

func packTracks(source layer, strength *track, put func(int, float64)) error {
	lifetime := source.OverLifetime

	const dragIndex, minimumTrackValue = 2, -1e6

	tracks := []*track{lifetime.Size, lifetime.Rotation, lifetime.Velocity.Drag, strength}
	for index, property := range tracks {
		limit := minimumTrackValue
		if index == dragIndex {
			limit = 0
		}

		if err := validateTrack(property, limit); err != nil {
			return err
		}
	}

	if err := validateGradient(lifetime.Color.Keys); err != nil {
		return err
	}

	for index := range particledata.Samples {
		age := float64(index) / (particledata.Samples - 1)

		const channels = 4

		offset := (particledata.ConfigVectors + 2*index) * channels

		for component, property := range tracks {
			fallback := 0.0
			if component == 0 {
				fallback = 1
			}

			put(offset+component, sampledTrack(property, age, fallback))
		}

		for component, value := range gradient(lifetime.Color.Keys, age) {
			put(offset+4+component, value)
		}
	}

	return nil
}

func shapeValues(source shape) ([4]float64, float64, error) {
	var surface float64
	if source.EmitFrom == "surface" {
		surface = 1
	} else if source.EmitFrom != "volume" {
		return [4]float64{}, 0, errDocument
	}

	if source.ArcMode != "" && source.ArcMode != "random" {
		return [4]float64{}, 0, fmt.Errorf("arc mode %q: %w", source.ArcMode, errUnsupported)
	}

	switch source.Kind {
	case "point":
		return [4]float64{}, surface, nil
	case "circle":
		if between(source.Radius, 0, 1e6) && between(source.InnerRadius, 0, source.Radius) && between(source.Arc, 0, 360) {
			return [4]float64{1, source.Radius, source.InnerRadius, source.Arc * math.Pi / 180}, surface, nil
		}
	case coneShape:
		if between(source.Radius, 0, 1e6) && between(source.Direction, -1e6, 1e6) && between(source.Spread, 0, 360) {
			return [4]float64{2, source.Radius, source.Direction * math.Pi / 180, source.Spread * math.Pi / 180}, surface, nil
		}
	case "rect":
		if between(source.Width, 0, 1e6) && between(source.Height, 0, 1e6) {
			return [4]float64{3, source.Width, source.Height}, surface, nil
		}
	case "edge":
		if between(source.Length, 0, 1e6) {
			return [4]float64{4, source.Length}, surface, nil
		}
	default:
		return [4]float64{}, 0, fmt.Errorf("spawn shape %q: %w", source.Kind, errUnsupported)
	}

	return [4]float64{}, 0, errDocument
}

func between(value, low, high float64) bool { return value >= low && value <= high }
