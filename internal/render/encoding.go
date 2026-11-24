package render

import (
	"encoding/binary"
	"math"

	"github.com/karty-game/ashura/internal/impostordata"
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
	// MaterialUniformSize is the padded layout shared in size by the scene materials.
	MaterialUniformSize = 112
	decalLayerOffset    = 96
	// FrameUniformBinding is the shared camera/light buffer in each material group.
	FrameUniformBinding = 8
	// FrameUniformSize is a view matrix, camera/count vec4, and eight light records.
	FrameUniformSize = 464
	// MaxLights bounds both materials' light arrays.
	MaxLights        = 8
	frameLightOffset = 20
	lightFloats      = 12
	floatSize        = 4
)

const diffuseLightmapMarker = 2

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

// Uniforms encodes only the material data owned by one draw.
func Uniforms(draw DrawCall) [MaterialUniformSize]byte {
	var values [decalLayerOffset / floatSize]float32
	if draw.Source {
		sourceUniforms(&values, draw)
	} else {
		standardUniforms(&values, draw)
	}

	var data [MaterialUniformSize]byte
	encodeUniform(data[:], values[:])

	if draw.Impostor == nil {
		binary.LittleEndian.PutUint32(data[decalLayerOffset:], draw.DecalLayers)
	}

	return data
}

func standardUniforms(values *[decalLayerOffset / floatSize]float32, draw DrawCall) {
	copy(values[:4], draw.Color[:])

	if draw.Impostor != nil {
		if draw.Impostor.Mode == impostordata.ModeOctahedral {
			copy(values[4:8], draw.ImpostorQuality[:])
		}

		return
	}

	values[4] = draw.Roughness
	if draw.Normal != nil {
		values[5] = draw.NormalStrength
	}

	values[6], values[7] = draw.UVScale[0], draw.UVScale[1]
	if draw.AO != nil {
		values[8] = draw.AOStrength
	}

	values[9] = draw.ReflectionStrength
	if draw.Lightmap != nil {
		values[10] = draw.LightmapScale
		if draw.LightmapDiffuse {
			values[15] = diffuseLightmapMarker + float32(draw.LightmapEncoding)
		}
	}

	if draw.LightmapDebug {
		values[11] = 1
	}

	copy(values[12:15], draw.ProbeOrigin[:])
	copy(values[16:19], draw.ProbeMinimum[:])

	if draw.AlphaMask {
		values[19], values[23] = draw.AlphaCutoff, 1
	}

	copy(values[20:23], draw.ProbeMaximum[:])
}

// sourceUniforms packs RNM/SSBump settings without Standard shading parameters.
func sourceUniforms(values *[decalLayerOffset / floatSize]float32, draw DrawCall) {
	copy(values[4:8], draw.Color[:])

	if draw.Normal != nil {
		values[1] = draw.SSBumpStrength
	}

	if draw.HalfLambert {
		values[2] = 1
	}

	if draw.RNM[0] != nil {
		values[3] = draw.RNMScale
		values[10] = float32(draw.RNMEncoding)
	}

	values[8], values[9] = draw.UVScale[0], draw.UVScale[1]
	for index, color := range draw.AmbientColors {
		const ambientOffset = 12
		copy(values[ambientOffset+index*floatSize:ambientOffset+(index+1)*floatSize], color[:])
	}
}

func encodeUniform(data []byte, values []float32) {
	for index, value := range values {
		binary.LittleEndian.PutUint32(data[index*floatSize:], math.Float32bits(value))
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
