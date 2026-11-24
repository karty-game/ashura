package render

import (
	"encoding/binary"
	"math"

	"github.com/karty-game/ashura/internal/impostordata"
	"github.com/karty-game/ashura/internal/texturedata"
)

// Span describes an aligned byte interval to upload. Intervals never include
// unchanged matrices, so sparse object edits do not overwrite unrelated data.
type Span struct{ Offset, End int }

// InstanceState retains packed matrices and compares each bounded component
// snapshot. Only changed matrices are encoded; unchanged frames emit no spans.
type InstanceState struct {
	Data    []byte
	Dirty   []Span
	models  []InstanceTransform
	count   int
	owner   any
	version uint64
}

// Update prepares one draw's matrices. A non-instanced draw uses its model.
func (state *InstanceState) Update(draw DrawCall) {
	count := max(len(draw.Instances), 1)

	previous := state.count

	state.Dirty = state.Dirty[:0]
	if draw.InstanceVersion != 0 && state.owner == draw.InstanceOwner && state.version == draw.InstanceVersion && previous == count {
		return
	}

	trusted := previous > 0 && draw.InstanceVersion != 0 && state.owner == draw.InstanceOwner && state.version+1 == draw.InstanceVersion
	if cap(state.models) < count {
		models := make([]InstanceTransform, count)
		copy(models, state.models)
		state.models = models
	} else {
		state.models = state.models[:count]
	}

	if cap(state.Data) < count*InstanceStride {
		data := make([]byte, count*InstanceStride)
		copy(data, state.Data)
		state.Data = data
	} else {
		state.Data = state.Data[:count*InstanceStride]
	}

	if trusted {
		for _, index := range draw.ChangedInstances {
			if index < count {
				state.encode(draw, index)
			}
		}
	} else {
		for index := range count {
			model := instanceAt(draw, index)
			if index < previous && state.models[index] == model {
				continue
			}

			state.encode(draw, index)
		}
	}

	state.owner, state.version = draw.InstanceOwner, draw.InstanceVersion
	state.count = count
}

func instanceAt(draw DrawCall, index int) InstanceTransform {
	if len(draw.Instances) != 0 {
		return draw.Instances[index]
	}

	return InstanceTransform{Model: draw.Transform}
}

// Count is the current draw's instance count.
func (state *InstanceState) Count() int { return state.count }

// All marks the full payload for a newly allocated/grown GPU buffer.
func (state *InstanceState) All() { state.Dirty = append(state.Dirty[:0], Span{0, len(state.Data)}) }

func (state *InstanceState) encode(draw DrawCall, index int) {
	model := instanceAt(draw, index)
	state.models[index] = model

	start := index * InstanceStride
	for field, value := range model.Model {
		binary.LittleEndian.PutUint32(state.Data[start+field*4:], math.Float32bits(value))
	}

	end := start + InstanceStride

	last := len(state.Dirty) - 1
	if last >= 0 && state.Dirty[last].End == start {
		state.Dirty[last].End = end
	} else {
		state.Dirty = append(state.Dirty, Span{start, end})
	}
}

type uniformKey struct {
	impostorQuality               [4]float32
	color                         [4]float32
	uv                            [2]float32
	probeOrigin, probeMinimum     [3]float32
	probeMaximum                  [3]float32
	roughness, normal, ao, ssbump float32
	reflection                    float32
	alphaCutoff                   float32
	alphaMask                     bool
	lightmap                      float32
	lightmapEncoding              texturedata.RNMEncoding
	lightmapDiffuse               bool
	rnmEncoding                   texturedata.RNMEncoding
	lightmapDebug                 bool
	source, halfLambert           bool
	ambient                       [3][4]float32
}

// UniformState skips encoding and uploading unchanged material records.
type UniformState struct {
	key   uniformKey
	Data  [MaterialUniformSize]byte
	ready bool
}

// Update encodes only when shader-visible material inputs change.
func (state *UniformState) Update(draw DrawCall) bool {
	key := uniformKey{
		color:       draw.Color,
		uv:          draw.UVScale,
		probeOrigin: draw.ProbeOrigin, probeMinimum: draw.ProbeMinimum, probeMaximum: draw.ProbeMaximum,
		reflection: draw.ReflectionStrength,
		source:     draw.Source,
	}

	key.color[3] = 0
	key.applyMaterial(draw)

	if state.ready && state.key == key {
		return false
	}

	state.Data = Uniforms(draw)
	state.key, state.ready = key, true

	return true
}

func (key *uniformKey) applyMaterial(draw DrawCall) {
	if draw.Source {
		key.halfLambert = draw.HalfLambert
		if draw.RNM[0] != nil {
			key.lightmap = draw.RNMScale
			key.rnmEncoding = draw.RNMEncoding
		} else {
			key.ambient = draw.AmbientColors
		}

		for index := range key.ambient {
			key.ambient[index][3] = 0
		}

		if draw.Normal != nil {
			key.ssbump = draw.SSBumpStrength
		}

		return
	}

	if draw.Impostor != nil {
		if draw.Impostor.Mode == impostordata.ModeOctahedral {
			key.impostorQuality = draw.ImpostorQuality
		}

		key.uv = [2]float32{}

		return
	}

	key.roughness = draw.Roughness
	if draw.AlphaMask {
		key.alphaMask, key.alphaCutoff = true, draw.AlphaCutoff
		key.color[3] = draw.Color[3]
	}

	if draw.Lightmap != nil {
		key.lightmap = draw.LightmapScale
		key.lightmapEncoding, key.lightmapDiffuse = draw.LightmapEncoding, draw.LightmapDiffuse
	}

	key.lightmapDebug = draw.LightmapDebug
	key.reflection = draw.ReflectionStrength

	key.probeOrigin, key.probeMinimum, key.probeMaximum = draw.ProbeOrigin, draw.ProbeMinimum, draw.ProbeMaximum
	if draw.Normal != nil {
		key.normal = draw.NormalStrength
	}

	if draw.AO != nil {
		key.ao = draw.AOStrength
	}
}

// FrameUniformState owns one camera and light buffer for the forward scene.
type FrameUniformState struct {
	key   [FrameUniformSize / floatSize]float32
	Data  [FrameUniformSize]byte
	ready bool
}

// Update returns whether to upload the shared forward-pass buffer.
func (state *FrameUniformState) Update(frame Frame) bool {
	var values [FrameUniformSize / floatSize]float32
	copy(values[:16], frame.ViewProjection[:])
	copy(values[16:19], frame.CameraPosition[:])
	count := min(len(frame.Lights), MaxLights)

	values[19] = float32(count)
	for index, light := range frame.Lights[:count] {
		offset := frameLightOffset + index*lightFloats
		copy(values[offset:offset+3], light.Direction[:])
		values[offset+3] = float32(light.Kind)
		copy(values[offset+4:offset+7], light.Color[:])

		values[offset+7] = light.Intensity
		if light.Kind != DirectionalLightKind {
			copy(values[offset+8:offset+11], light.GroundColor[:])
		}
	}

	if state.ready && state.key == values {
		return false
	}

	encodeUniform(state.Data[:], values[:])
	state.key, state.ready = values, true

	return true
}

// Invalidate forces the next update after a failed upload.
func (state *FrameUniformState) Invalidate() { state.ready = false }
