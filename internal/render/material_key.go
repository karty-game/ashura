package render

import (
	"github.com/karty-game/karty-webgpu/internal/impostordata"
	"github.com/karty-game/karty-webgpu/internal/texturedata"
)

// MaterialKey retains shader-visible identities and numbers inline (<=128 bytes).
// Map and numeric lanes are shared by mutually exclusive shader layouts; no
// interface boxing or allocation is needed while collecting frame batches.
type MaterialKey struct {
	Impostor                                 *impostordata.Document
	Maps                                     [5]*texturedata.Texture
	Values                                   [16]float32
	Source, HalfLambert, Cull, LightmapDebug bool
}

// EffectiveMaterial ignores fields not consumed by the selected shader.
func EffectiveMaterial(draw DrawCall) MaterialKey {
	key := MaterialKey{Impostor: draw.Impostor, Source: draw.Source, Cull: draw.CullBackfaces}
	key.Maps[0], key.Maps[1] = draw.Albedo, draw.Normal
	copy(key.Values[:3], draw.Color[:3])

	if draw.Impostor != nil {
		if draw.Impostor.Mode == impostordata.ModeOctahedral {
			copy(key.Values[3:7], draw.ImpostorQuality[:])
		}

		return key
	}

	copy(key.Values[3:5], draw.UVScale[:])

	if draw.Source {
		key.HalfLambert = draw.HalfLambert

		if draw.Normal != nil {
			key.Values[5] = draw.SSBumpStrength
		}

		if draw.RNM[0] != nil {
			copy(key.Maps[2:], draw.RNM[:])
			key.Values[6] = draw.RNMScale
			key.Values[7] = float32(draw.RNMEncoding)
		} else {
			for index := range draw.AmbientColors {
				copy(key.Values[7+index*3:], draw.AmbientColors[index][:3])
			}
		}

		return key
	}

	key.Maps[2], key.Maps[3], key.Maps[4] = draw.AO, draw.Reflection, draw.Lightmap
	key.Values[5], key.Values[6] = draw.Roughness, draw.ReflectionStrength

	if draw.Normal != nil {
		key.Values[7] = draw.NormalStrength
	}

	if draw.AO != nil {
		key.Values[8] = draw.AOStrength
	}

	if draw.Lightmap != nil {
		key.Values[9] = draw.LightmapScale
		if draw.LightmapDiffuse {
			key.Values[10] = diffuseLightmapMarker + float32(draw.LightmapEncoding)
		}
	}

	if draw.AlphaMask {
		key.Values[11], key.Values[12], key.Values[13] = 1, draw.AlphaCutoff, draw.Color[3]
	}

	key.LightmapDebug = draw.LightmapDebug

	return key
}
