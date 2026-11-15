package engine

import (
	"errors"
	"testing"

	"github.com/karty-game/karty-webgpu/internal/meshdata"
	"github.com/karty-game/karty-webgpu/internal/render"
	"github.com/karty-game/karty-webgpu/internal/texturedata"
)

func TestCalibratedLightmapsUseCoherentHardwareTransfer(t *testing.T) {
	t.Parallel()

	geometry, err := meshdata.New(make([]byte, 3*render.VertexStride), make([]byte, 3*render.IndexSize))
	if err != nil {
		t.Fatal(err)
	}

	prepared, err := geometry.WithLightmapUV(make([]byte, 3*render.LightmapUVStride))
	if err != nil {
		t.Fatal(err)
	}

	linear := texturedata.Solid([4]byte{32, 48, 64, 255}, texturedata.Linear)
	srgb := texturedata.Solid([4]byte{32, 48, 64, 255}, texturedata.SRGB)

	draw := render.DrawCall{Source: true, Prepared: prepared, RNMScale: 1, RNM: [3]*texturedata.Texture{srgb, srgb, srgb}}
	if err = validateRNMLightmap(draw); err != nil {
		t.Fatal("coherent hardware sRGB RNM rejected", err)
	}

	for _, invalid := range []render.DrawCall{
		{Source: true, Prepared: prepared, RNMScale: 1, RNM: [3]*texturedata.Texture{nil, srgb, srgb}},
		{Source: true, Prepared: prepared, RNMScale: 1, RNM: [3]*texturedata.Texture{linear, srgb, srgb}},
		{Source: true, Prepared: prepared, RNMScale: 1, RNM: [3]*texturedata.Texture{srgb, srgb, srgb}, RNMEncoding: texturedata.RNMRGBE},
	} {
		if err = validateRNMLightmap(invalid); !errors.Is(err, errInvalidLightmap) {
			t.Fatal("invalid transfer tuple admitted", err)
		}
	}

	rgb := render.DrawCall{Prepared: prepared, Lightmap: srgb, LightmapScale: 1, LightmapDiffuse: true}
	if err = validateRGBLightmap(rgb); err != nil {
		t.Fatal("calibrated hardware sRGB RGB map rejected", err)
	}

	rgb.LightmapEncoding = texturedata.RNMRGBE
	if err = validateRGBLightmap(rgb); !errors.Is(err, errInvalidLightmap) {
		t.Fatal("sRGB RGBE mantissas admitted", err)
	}

	rgb.LightmapEncoding, rgb.LightmapDiffuse = texturedata.RNMLinear, false
	if err = validateRGBLightmap(rgb); !errors.Is(err, errInvalidLightmap) {
		t.Fatal("legacy noncalibrated RGB transfer changed", err)
	}
}
