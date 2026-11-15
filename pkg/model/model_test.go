//go:build !js

package model

import (
	"encoding/binary"
	"errors"
	"math"
	"testing"

	"github.com/karty-game/karty-webgpu/internal/modelbuild"
	"github.com/karty-game/karty-webgpu/internal/modelbuild/testfixture"
	"github.com/karty-game/karty-webgpu/internal/modeldata"
	"github.com/karty-game/karty-webgpu/internal/texturedata"
	"github.com/karty-game/karty-webgpu/pkg/engine"
	"github.com/karty-game/karty-webgpu/pkg/material"
)

func TestRootTransformAdmissionIsAtomicAndPreservesOrientation(t *testing.T) {
	t.Parallel()

	app := engine.NewApp(engine.AppConfig{})
	defer func() { _ = app.Close() }()

	asset, _ := fixtureAsset(t, false)

	model, err := asset.Load(app)
	if err != nil {
		t.Fatal(err)
	}

	world := engine.NewWorld()
	for _, transform := range []engine.Transform{
		{Scale: engine.Vector3{X: -1, Y: 1, Z: 1}},
		{Scale: engine.Vector3{X: 1, Y: 0, Z: 1}},
		{Position: engine.Vector3{X: float32(math.NaN())}},
	} {
		if _, err := model.Instantiate(world, transform); err == nil || world.Len() != 0 || len(model.instances) != 0 {
			t.Fatal("invalid root transform changed the world")
		}
	}

	for _, transform := range []engine.Transform{{}, {Scale: engine.Vector3{X: -1, Y: -1, Z: 1}}} {
		instance, err := model.Instantiate(world, transform)
		if err != nil {
			t.Fatal(err)
		}

		instance.Remove()
	}
}

func fixtureAsset(t *testing.T, textured bool) (Asset, []byte) {
	t.Helper()

	document, data := testfixture.Triangle()
	if textured {
		data = testfixture.EmbedTexture(document, data)
	}

	result, err := modelbuild.Compile(testfixture.GLB(document, data), "triangle.glb")
	if err != nil {
		t.Fatal(err)
	}

	return Prepared("triangle.glb", result.Data), result.Data
}

func TestSharedInstantiationRemovalAndExplicitUnload(t *testing.T) {
	t.Parallel()

	app := engine.NewApp(engine.AppConfig{})
	defer func() { _ = app.Close() }()

	asset, source := fixtureAsset(t, true)

	model, err := asset.Load(app)
	if err != nil {
		t.Fatal(err)
	}

	world := engine.NewWorld()

	first, err := model.Instantiate(world, engine.DefaultTransform())
	if err != nil {
		t.Fatal(err)
	}

	second, err := model.Instantiate(world, engine.Transform{Position: engine.Vector3{X: 2}})
	if err != nil {
		t.Fatal(err)
	}

	firstPart, _ := engine.Get[part](world, first.Entities()[0])

	secondPart, _ := engine.Get[part](world, second.Entities()[0])
	if firstPart.RuntimePreparedMesh() != secondPart.RuntimePreparedMesh() ||
		firstPart.RuntimeMeshIdentity() != secondPart.RuntimeMeshIdentity() {
		t.Fatal("model instances did not share immutable prepared geometry")
	}

	firstMaterial, _ := engine.Get[material.Material](world, first.Entities()[0])

	secondMaterial, _ := engine.Get[material.Material](world, second.Entities()[0])
	if firstMaterial.AlbedoTexture != secondMaterial.AlbedoTexture || !firstMaterial.CullBackfaces {
		t.Fatal("shared texture or glTF sidedness lost")
	}

	decoded, err := modeldata.Decode(source)
	if err != nil {
		t.Fatal(err)
	}

	before := append([]byte(nil), firstPart.geometry.Vertices()...)

	decoded.Parts[0].Vertices[0] ^= 255
	if firstPart.geometry.Vertices()[0] != before[0] {
		t.Fatal("loaded model retained caller-owned mutable container")
	}

	if err := model.Unload(); !errors.Is(err, ErrInUse) || !errors.Is(err, engine.ErrInUse) {
		t.Fatalf("live instances must return the public in-use category: %v", err)
	}

	if !first.Remove() || first.Remove() || second.Len() != 1 || !world.Alive(second.Entities()[0]) {
		t.Fatal("removing one instance damaged surviving shared user")
	}

	if err := model.Unload(); err == nil {
		t.Fatal("unloaded source while second instance remained")
	}

	second.Remove()

	image := model.textures[0]
	if err := model.Unload(); err != nil {
		t.Fatal(err)
	}

	if texturedata.Valid(image) {
		t.Fatal("unload retained texture ownership")
	}

	if err := model.Unload(); err != nil {
		t.Fatal(err)
	}

	if _, err := model.Instantiate(world, engine.DefaultTransform()); !errors.Is(err, ErrClosed) || errors.Is(err, ErrInUse) {
		t.Fatalf("unloaded model must return closed, not in-use: %v", err)
	}
}

func TestDirectWorldRemovalDoesNotRetainInstanceHistory(t *testing.T) {
	t.Parallel()

	app := engine.NewApp(engine.AppConfig{})
	defer func() { _ = app.Close() }()

	asset, _ := fixtureAsset(t, false)

	model, err := asset.Load(app)
	if err != nil {
		t.Fatal(err)
	}

	world := engine.NewWorld()
	for range 100 {
		instance, createErr := model.Instantiate(world, engine.DefaultTransform())
		if createErr != nil {
			t.Fatal(createErr)
		}

		if len(model.instances) != 1 {
			t.Fatalf("retained %d historical instances", len(model.instances))
		}

		for _, entity := range instance.Entities() {
			world.Remove(entity)
		}
	}

	if err := model.Unload(); err != nil {
		t.Fatal(err)
	}
}

func TestClosedOwnerAndInvalidPreparedBatchDoNotMutateWorld(t *testing.T) {
	t.Parallel()

	app := engine.NewApp(engine.AppConfig{})
	asset, source := fixtureAsset(t, false)

	model, err := asset.Load(app)
	if err != nil {
		t.Fatal(err)
	}

	geometry := model.parts[0].geometry

	if err := app.Close(); err != nil {
		t.Fatal(err)
	}

	if len(geometry.Vertices()) != 0 || model.loaded {
		t.Fatal("App.Close retained owned model geometry")
	}

	world := engine.NewWorld()
	if _, err := model.Instantiate(world, engine.DefaultTransform()); err == nil || world.Len() != 0 {
		t.Fatal("closed-owner model instantiated entities")
	}

	if _, err := asset.Load(app); err == nil {
		t.Fatal("asset loaded into closed App")
	}

	if err := model.Unload(); err != nil {
		t.Fatal(err)
	}

	if _, err := Prepared("empty", nil).Load(engine.NewApp(engine.AppConfig{})); err == nil {
		t.Fatal("empty prepared model loaded")
	}

	document, err := modeldata.Decode(source)
	if err != nil {
		t.Fatal(err)
	}

	document.Parts = append(document.Parts, document.Parts[0])

	prepared, err := modeldata.Encode(document)
	if err != nil {
		t.Fatal(err)
	}

	corrupt, err := modeldata.Decode(prepared)
	if err != nil {
		t.Fatal(err)
	}

	binary.LittleEndian.PutUint32(corrupt.Parts[1].Indices[8:], 3)

	if _, err := Prepared("bad late primitive", prepared).Load(engine.NewApp(engine.AppConfig{})); err == nil {
		t.Fatal("invalid late primitive escaped complete admission")
	}
}
