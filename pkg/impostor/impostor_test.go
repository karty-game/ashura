//go:build !js

package impostor_test

import (
	"encoding/binary"
	"errors"
	"math"
	"testing"

	"github.com/karty-game/karty-webgpu/internal/impostordata"
	"github.com/karty-game/karty-webgpu/pkg/assetbuild"
	"github.com/karty-game/karty-webgpu/pkg/engine"
	"github.com/karty-game/karty-webgpu/pkg/impostor"
)

func fixtureAsset(t *testing.T) []byte {
	t.Helper()

	views := make([]impostordata.View, 4)
	for index := range views {
		views[index] = impostordata.View{Right: [3]float32{1, 0, 0}, Up: [3]float32{0, 1, 0}, Outward: [3]float32{0, 0, 1}}
	}

	vertices := make([]byte, 4*32)

	for index, point := range [][3]float32{{-1, -1, 0}, {1, -1, 0}, {1, 1, 0}, {-1, 1, 0}} {
		for channel, value := range point {
			binary.LittleEndian.PutUint32(vertices[index*32+channel*4:], math.Float32bits(value))
		}
	}

	indices := make([]byte, 6*4)
	for index, value := range []uint32{0, 1, 2, 0, 2, 3} {
		binary.LittleEndian.PutUint32(indices[index*4:], value)
	}

	color := make([]byte, 8*8*4)

	normal := make([]byte, len(color))
	for offset := 0; offset < len(color); offset += 4 {
		copy(color[offset:], []byte{50, 180, 50, 255})
		copy(normal[offset:], []byte{128, 128, 255, 128})
	}

	colorData, err := assetbuild.EncodeTexture(
		assetbuild.Image{Name: "Color", Source: "color.png", Usage: "albedo", Width: 8, Height: 8, Levels: [][]byte{color}},
	)
	if err != nil {
		t.Fatal(err)
	}

	normalData, err := assetbuild.EncodeTexture(
		assetbuild.Image{Name: "Normal", Source: "normal.png", Usage: "normal", Width: 8, Height: 8, Levels: [][]byte{normal}},
	)
	if err != nil {
		t.Fatal(err)
	}

	document := impostordata.Document{
		Source:          "tree.glb",
		Center:          [3]float32{0, 1, 0},
		Radius:          2,
		Roughness:       .8,
		Azimuths:        2,
		Elevations:      2,
		ElevationMin:    -.5,
		ElevationMax:    .5,
		AtlasWidth:      8,
		AtlasHeight:     8,
		Columns:         2,
		CellSize:        4,
		Resolution:      2,
		Padding:         1,
		MipLevels:       1,
		SourceTriangles: 12345,
		Views:           views,
		Vertices:        vertices,
		Indices:         indices,
		Color:           colorData,
		NormalDepth:     normalData,
	}

	data, err := impostordata.Encode(document)
	if err != nil {
		t.Fatal(err)
	}

	return data
}

func TestAppOwnedLifetimeAndPreparedInstances(t *testing.T) {
	t.Parallel()

	app := engine.NewApp(engine.AppConfig{})

	t.Cleanup(func() { _ = app.Close() })

	asset, err := impostor.Prepared("tree.glb", fixtureAsset(t)).Load(app)
	if err != nil {
		t.Fatal(err)
	}

	if asset.Source() != "tree.glb" || asset.Triangles() != 12345 || asset.ProxyTriangles() != 2 ||
		asset.Center() != (engine.Vector3{Y: 1}) {
		t.Fatal("lost authored metadata")
	}

	world := engine.NewWorld()

	first, err := asset.Instantiate(world, engine.Transform{Position: engine.Vector3{X: 3}})
	if err != nil {
		t.Fatal(err)
	}

	second, err := asset.Instantiate(world, engine.DefaultTransform())
	if err != nil {
		t.Fatal(err)
	}

	if first.Entity() == second.Entity() || world.Transform(first.Entity()).Position.X != 3 {
		t.Fatal("instances did not retain independent transforms")
	}

	if err := asset.Unload(); !errors.Is(err, impostor.ErrInUse) || !errors.Is(err, engine.ErrInUse) {
		t.Fatalf("live instances must return the public in-use category: %v", err)
	}

	if !first.Remove() || first.Remove() {
		t.Fatal("instance removal not idempotent")
	}

	world.Remove(second.Entity())

	if err := asset.Unload(); err != nil {
		t.Fatal(err)
	}

	if _, err := asset.Instantiate(
		world,
		engine.DefaultTransform(),
	); !errors.Is(err, impostor.ErrClosed) ||
		errors.Is(err, impostor.ErrInUse) {
		t.Fatalf("unloaded impostor must return closed, not in-use: %v", err)
	}

	if err := asset.Unload(); err != nil {
		t.Fatal(err)
	}
}

func TestAppCloseInvalidatesImpostorAndRejectsBadTransforms(t *testing.T) {
	t.Parallel()

	app := engine.NewApp(engine.AppConfig{})

	asset, err := impostor.Prepared("tree.glb", fixtureAsset(t)).Load(app)
	if err != nil {
		t.Fatal(err)
	}

	world := engine.NewWorld()
	for _, transform := range []engine.Transform{{Scale: engine.Vector3{X: 1}}, {Scale: engine.Vector3{X: -1, Y: 1, Z: 1}}, {Position: engine.Vector3{X: float32(math.NaN())}}} {
		if _, err := asset.Instantiate(world, transform); err == nil {
			t.Fatal("accepted invalid root transform")
		}
	}

	if _, err := asset.Instantiate(nil, engine.DefaultTransform()); err == nil {
		t.Fatal("accepted nil World")
	}

	if err := app.Close(); err != nil {
		t.Fatal(err)
	}

	if _, err := asset.Instantiate(world, engine.DefaultTransform()); err == nil {
		t.Fatal("App.Close retained usable impostor")
	}

	if _, err := impostor.Prepared("tree.glb", fixtureAsset(t)).Load(app); err == nil {
		t.Fatal("loaded into closed App")
	}
}

func TestCompleteAdmissionRejectsLatePayloadBeforeResources(t *testing.T) {
	t.Parallel()
	data := fixtureAsset(t)

	document, err := impostordata.Decode(data)
	if err != nil {
		t.Fatal(err)
	}

	document.NormalDepth = []byte("not a prepared KTX2")

	bad, err := impostordata.Encode(document)
	if err != nil {
		t.Fatal(err)
	}

	app := engine.NewApp(engine.AppConfig{})

	t.Cleanup(func() { _ = app.Close() })

	if _, err := impostor.Prepared("bad", bad).Load(app); err == nil {
		t.Fatal("accepted bad second texture")
	}

	if _, err := impostor.Prepared("good", data).Load(app); err != nil {
		t.Fatal("bad admission poisoned App resources")
	}
}
