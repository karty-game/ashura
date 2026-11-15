package engine

import (
	"errors"
	"math"
	"testing"

	"github.com/karty-game/karty-webgpu/internal/meshdata"
	"github.com/karty-game/karty-webgpu/internal/render"
	"github.com/karty-game/karty-webgpu/internal/texturedata"
	"github.com/karty-game/karty-webgpu/pkg/texture"
)

type rnmFixture struct {
	sourceBindingFixture

	maps *texture.RNMLightmap
}

func (value rnmFixture) RuntimeRNMLightmap() *texture.RNMLightmap { return value.maps }

func TestRNMReferencesAreSnapshotAndUnloadProtected(t *testing.T) {
	t.Parallel()

	app := NewApp(AppConfig{})
	defer func() { _ = app.Close() }()

	maps := &texture.RNMLightmap{Scale: 2}

	for index := range maps.Textures {
		image, err := app.NewTexture(texture.Data{Width: 1, Height: 1, ColorSpace: texture.Linear, Levels: [][]byte{{1, 2, 3, 255}}})
		if err != nil {
			t.Fatal(err)
		}

		maps.Textures[index] = image
	}

	world := NewWorld()
	entity := world.Add(DefaultTransform(), fixtureTriangle(), rnmFixture{maps: maps})
	app.scene = world

	draw, ok := drawForEntity(world, entity)

	if !ok || draw.RNM != maps.Textures || draw.RNMScale != 2 {
		t.Fatal("RNM provider not captured")
	}

	maps.Scale = 3
	maps.Encoding = texture.RNMRGBE
	changed, _ := drawForEntity(world, entity)

	if draw.RNMScale != 2 || changed.RNMScale != 3 || draw.RNMEncoding != texture.RNMLinear || changed.RNMEncoding != texture.RNMRGBE {
		t.Fatal("draw does not retain an independent scale snapshot")
	}

	for _, image := range maps.Textures {
		if err := app.UnloadTexture(image); !errors.Is(err, errTextureInUse) {
			t.Fatalf("RNM texture could unload: %v", err)
		}
	}

	world.Remove(entity)

	if err := app.UnloadTextures(maps.Textures[:]); err != nil {
		t.Fatal(err)
	}
}

func TestRNMAdmissionRequiresCompleteLinearTupleAndUV1(t *testing.T) {
	t.Parallel()

	linear := texturedata.Solid([4]byte{1, 2, 3, 255}, texturedata.Linear)
	srgb := texturedata.Solid([4]byte{1, 2, 3, 255}, texturedata.SRGB)
	draw := render.DrawCall{Source: true, RNM: [3]*texturedata.Texture{linear, linear, linear}, RNMScale: 2}

	if err := validateRNMLightmap(draw); !errors.Is(err, errInvalidLightmap) {
		t.Fatal("unprepared UVs accepted")
	}

	geometry, err := meshdata.New(make([]byte, 3*render.VertexStride), make([]byte, 3*render.IndexSize))
	if err != nil {
		t.Fatal(err)
	}

	prepared, err := geometry.WithLightmapUV(make([]byte, 3*render.LightmapUVStride))
	if err != nil {
		t.Fatal(err)
	}

	draw.Prepared = prepared
	if err := validateRNMLightmap(draw); err != nil {
		t.Fatalf("valid RNM admission: %v", err)
	}

	draw.RNMEncoding = texture.RNMRGBE
	if err := validateRNMLightmap(draw); err != nil {
		t.Fatalf("valid RGBE admission: %v", err)
	}

	for _, value := range []render.DrawCall{
		{Source: true, RNM: [3]*texturedata.Texture{linear, nil, linear}, RNMScale: 2},
		{Source: true, RNM: [3]*texturedata.Texture{linear, srgb, linear}, RNMScale: 2},
		{Source: true, RNM: [3]*texturedata.Texture{linear, linear, linear}, RNMScale: float32(math.NaN())},
		{Source: true, RNM: [3]*texturedata.Texture{linear, linear, linear}, RNMScale: 2, RNMEncoding: 255},
	} {
		value.Prepared = prepared

		if err := validateRNMLightmap(value); !errors.Is(err, errInvalidLightmap) {
			t.Fatal("malformed RNM tuple accepted")
		}
	}
}
