// Package modeldata describes the versioned, GPU-ready static-model container.
// It contains no GLB, JSON, image codecs or geometry transformations.
package modeldata

import (
	"errors"
	"math"
)

const (
	// MaxBytes bounds the complete prepared container before any allocation.
	MaxBytes = 128 << 20
	// MaxParts and MaxTextures bound model-owned resource counts.
	MaxParts        = 4096
	MaxTextures     = 64
	maxText         = 4096
	version         = 1
	maskVersion     = 2
	maskWords       = 2
	headerSize      = 24
	wordSize        = 4
	materialWords   = 11
	partHeaderWords = 3
	magic           = "KARTMDL\x00"
)

var (
	errFormat = errors.New("invalid prepared static model container")
	errBounds = errors.New("prepared static model exceeds resource bounds")
)

// TextureRole identifies the authored mip/color-space interpretation.
type TextureRole uint32

const (
	// Albedo contains sRGB color data.
	Albedo TextureRole = iota
	// Normal contains linear tangent-space normal data.
	Normal
	// Occlusion contains linear red-channel ambient occlusion.
	Occlusion
)

// Texture contains an already prepared KTX2 payload.
type Texture struct {
	Role TextureRole
	Data []byte
}

// Material is the supported opaque or alpha-masked Standard shading subset.
type Material struct {
	Color                                 [4]float32
	Roughness, NormalStrength, AOStrength float32
	Albedo, Normal, AO                    int32
	DoubleSided                           bool
	AlphaMask                             bool
	AlphaCutoff                           float32
}

// Part is one flattened static primitive in the exact renderer upload layout.
type Part struct {
	Name              string
	Vertices, Indices []byte
	Material          Material
}

// Document contains prepared parts and shared immutable textures.
type Document struct {
	Source   string
	Parts    []Part
	Textures []Texture
}

func finite(value float32) bool { return !math.IsNaN(float64(value)) && !math.IsInf(float64(value), 0) }

func validMaterial(value Material, textures []Texture) bool {
	for _, channel := range value.Color {
		if !finite(channel) || channel < 0 || channel > 1 {
			return false
		}
	}

	if !validAlpha(value) || !finite(value.Roughness) || value.Roughness < 0 || value.Roughness > 1 ||
		!finite(value.NormalStrength) || !finite(value.AOStrength) || value.AOStrength < 0 || value.AOStrength > 1 {
		return false
	}

	return validTexture(value.Albedo, Albedo, textures) && validTexture(value.Normal, Normal, textures) &&
		validTexture(value.AO, Occlusion, textures)
}

func validAlpha(value Material) bool {
	if !finite(value.AlphaCutoff) || value.AlphaCutoff < 0 || value.AlphaCutoff > 1 {
		return false
	}

	return value.AlphaMask || value.Color[3] == 1 && value.AlphaCutoff == 0
}

func validTexture(index int32, role TextureRole, textures []Texture) bool {
	return index == -1 || index >= 0 && int64(index) < int64(len(textures)) && textures[index].Role == role
}

func documentVersion(document Document) uint32 {
	for _, part := range document.Parts {
		if part.Material.AlphaMask {
			return maskVersion
		}
	}

	return version
}
