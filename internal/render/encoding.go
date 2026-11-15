package render

import (
	"encoding/binary"
	"math"

	"github.com/karty-game/karty-webgpu/internal/impostordata"
)

const (
	// VertexStride is tightly packed position, normal and UV data, in bytes.
	VertexStride = 32
	// NormalOffset is the vertex normal byte offset.
	NormalOffset = 12
	// UVOffset is the vertex texture coordinate byte offset.
	UVOffset = 24
	// UVLocation is the texture-coordinate vertex attribute location.
	UVLocation = 2
	// LightmapUVLocation and LightmapUVStride identify the prepared secondary stream.
	LightmapUVLocation = 7
	LightmapUVStride   = 8
	// IndexSize is the uint32 index width in bytes.
	IndexSize = 4
	// InstanceStride is the packed model matrix size in bytes.
	InstanceStride = 64
	// AlbedoBinding identifies the albedo texture slot.
	AlbedoBinding = 2
	// NormalBinding identifies the normal texture slot.
	NormalBinding = 3
	// OcclusionBinding identifies the linear ambient-occlusion texture slot.
	OcclusionBinding = 4
	// ReflectionBinding identifies the sRGB cubemap-atlas texture slot.
	ReflectionBinding = 5
	// RNMLightmapBinding starts the three Source directional irradiance slots.
	RNMLightmapBinding = 4
	// LightmapBinding and LightmapSamplerBinding bind linear clamped irradiance.
	LightmapBinding        = 6
	LightmapSamplerBinding = 7
	// StandardUniformSize is the Standard material's padded uniform layout.
	StandardUniformSize = 560
	// SourceUniformSize is the Source material's padded uniform layout.
	SourceUniformSize = 544
	// MaxLights bounds both materials' light arrays.
	MaxLights            = 8
	standardHeaderFloats = 44
	sourceHeaderFloats   = 40
	standardLightFloats  = 12
	sourceLightFloats    = 12
	floatSize            = 4
)

// AppendInstances encodes instance matrices as vertex-step GPU attributes.
const diffuseLightmapMarker = 2

func AppendInstances(data []byte, instances []InstanceTransform) []byte {
	for _, instance := range instances {
		for _, value := range instance.Model {
			data = appendFloat(data, value)
		}
	}

	return data
}

// AppendVertices encodes the common object-space vertex format without lighting.
func AppendVertices(data []byte, vertices []Vertex) []byte {
	for _, vertex := range vertices {
		for _, value := range vertex.Position {
			data = appendFloat(data, value)
		}

		for _, value := range vertex.Normal {
			data = appendFloat(data, value)
		}

		for _, value := range vertex.UV {
			data = appendFloat(data, value)
		}
	}

	return data
}

// UniformSize returns the material-specific uniform buffer size.
func UniformSize(source bool) uint32 {
	if source {
		return SourceUniformSize
	}

	return StandardUniformSize
}

// Uniforms encodes a draw without allocating. Only UniformSize(draw.Source)
// bytes are uploaded; the fixed return array fits either material layout.
func Uniforms(draw DrawCall, frame Frame) [StandardUniformSize]byte {
	var values [StandardUniformSize / floatSize]float32
	copy(values[:16], frame.ViewProjection[:])
	copy(values[20:24], draw.Color[:])

	header, stride := standardHeaderFloats, standardLightFloats

	if draw.Source {
		sourceUniforms(&values, draw, frame)

		header, stride = sourceHeaderFloats, sourceLightFloats
	} else {
		standardUniforms(&values, draw, frame)
	}

	for index, light := range frame.Lights[:min(len(frame.Lights), MaxLights)] {
		offset := header + index*stride
		copy(values[offset:offset+3], light.Direction[:])
		values[offset+3] = float32(light.Kind)
		copy(values[offset+4:offset+7], light.Color[:])

		values[offset+7] = light.Intensity
		if !draw.Source || light.Kind == PointLightKind {
			copy(values[offset+8:offset+11], light.GroundColor[:])
		}
	}

	var data [StandardUniformSize]byte
	for index, value := range values[:UniformSize(draw.Source)/floatSize] {
		binary.LittleEndian.PutUint32(data[index*floatSize:], math.Float32bits(value))
	}

	return data
}

// standardUniforms packs camera, roughness, normal and AO settings only.
func standardUniforms(values *[StandardUniformSize / floatSize]float32, draw DrawCall, frame Frame) {
	copy(values[16:19], frame.CameraPosition[:])
	values[19] = float32(min(len(frame.Lights), MaxLights))

	if draw.Impostor != nil {
		if draw.Impostor.Mode == impostordata.ModeOctahedral {
			copy(values[24:28], draw.ImpostorQuality[:])
		}

		return
	}

	values[24] = draw.Roughness
	if draw.Normal != nil {
		values[25] = draw.NormalStrength
	}

	values[26], values[27] = draw.UVScale[0], draw.UVScale[1]
	if draw.AO != nil {
		values[28] = draw.AOStrength
	}

	values[29] = draw.ReflectionStrength
	if draw.Lightmap != nil {
		values[30] = draw.LightmapScale
		if draw.LightmapDiffuse {
			values[35] = diffuseLightmapMarker + float32(draw.LightmapEncoding)
		}
	}

	if draw.LightmapDebug {
		values[31] = 1
	}

	copy(values[32:35], draw.ProbeOrigin[:])
	copy(values[36:39], draw.ProbeMinimum[:])

	if draw.AlphaMask {
		values[39], values[43] = draw.AlphaCutoff, 1
	}

	copy(values[40:43], draw.ProbeMaximum[:])
}

// sourceUniforms packs RNM/SSBump settings without Standard shading parameters.
func sourceUniforms(values *[StandardUniformSize / floatSize]float32, draw DrawCall, frame Frame) {
	values[16] = float32(min(len(frame.Lights), MaxLights))
	if draw.Normal != nil {
		values[17] = draw.SSBumpStrength
	}

	if draw.HalfLambert {
		values[18] = 1
	}

	if draw.RNM[0] != nil {
		values[19] = draw.RNMScale
		values[26] = float32(draw.RNMEncoding)
	}

	values[24], values[25] = draw.UVScale[0], draw.UVScale[1]
	for index, color := range draw.AmbientColors {
		const ambientOffset = 28
		copy(values[ambientOffset+index*floatSize:ambientOffset+(index+1)*floatSize], color[:])
	}
}

func appendFloat(data []byte, value float32) []byte {
	return binary.LittleEndian.AppendUint32(data, math.Float32bits(value))
}

// AppendIndices encodes the native indexed draw stream.
func AppendIndices(data []byte, indices []uint32) []byte {
	for _, index := range indices {
		data = binary.LittleEndian.AppendUint32(data, index)
	}

	return data
}
