//go:build !js

package model

import (
	"errors"
	"fmt"
	"math"
	"testing"

	"github.com/karty-game/ashura/internal/modeldata"
	"github.com/karty-game/ashura/pkg/engine"
	"github.com/karty-game/ashura/pkg/material"
)

func TestInstantiateRangeSelectsPreparedPartsAndPreservesLifetime(t *testing.T) {
	t.Parallel()

	app := engine.NewApp(engine.AppConfig{})
	defer func() { _ = app.Close() }()

	loaded := multipartFixture(t, app, 3)

	var err error

	world := engine.NewWorld()
	for _, bounds := range [][2]int{{-1, 1}, {0, 0}, {3, 1}, {1, math.MaxInt}, {0, -1}} {
		if _, err = loaded.InstantiateRange(
			world,
			engine.DefaultTransform(),
			bounds[0],
			bounds[1],
		); err == nil || world.Len() != 0 ||
			len(loaded.instances) != 0 {
			t.Fatal("invalid prepared part range changed world or lifetime")
		}
	}

	instance, err := loaded.InstantiateRange(world, engine.DefaultTransform(), 1, 1)
	if err != nil {
		t.Fatal(err)
	}

	source, ok := engine.Get[part](world, instance.Entities()[0])
	if !ok || instance.Len() != 1 || source.geometry != loaded.parts[1].geometry {
		t.Fatal("range did not select the existing prepared geometry")
	}

	if err = loaded.Unload(); err == nil {
		t.Fatal("model unloaded with live ranged instance")
	}

	instance.Remove()

	complete, err := loaded.Instantiate(world, engine.DefaultTransform())
	if err != nil || complete.Len() != 3 {
		t.Fatalf("full Instantiate changed behavior: %v", err)
	}

	complete.Remove()

	if err = loaded.Unload(); err != nil {
		t.Fatal(err)
	}

	if _, err = loaded.InstantiateRange(world, engine.DefaultTransform(), 0, 1); err == nil {
		t.Fatal("unloaded ranged geometry instantiated")
	}
}

func multipartFixture(t *testing.T, app *engine.App, count int) *Model {
	t.Helper()

	_, data := fixtureAsset(t, false)

	document, err := modeldata.Decode(data)
	if err != nil {
		t.Fatal(err)
	}

	original := document.Parts[0]
	document.Parts = nil

	for index := range count {
		part := original
		part.Name = fmt.Sprintf("part%d", index)
		document.Parts = append(document.Parts, part)
	}

	data, err = modeldata.Encode(document)
	if err != nil {
		t.Fatal(err)
	}

	loaded, err := Prepared("multipart model", data).Load(app)
	if err != nil {
		t.Fatal(err)
	}

	return loaded
}

func TestInstantiateWithMaterialAndMoveAllParts(t *testing.T) {
	t.Parallel()

	app := engine.NewApp(engine.AppConfig{})
	defer func() { _ = app.Close() }()

	model := multipartFixture(t, app, 2)
	world := engine.NewWorld()
	start := engine.DefaultTransform()

	var missing *material.SourceMaterial
	if _, err := model.InstantiateWithMaterial(world, start, missing); !errors.Is(err, engine.ErrInvalidComponent) || world.Len() != 0 {
		t.Fatalf("nil override created an instance or returned the wrong error: %v", err)
	}

	finish := material.DefaultSource()
	finish.HalfLambert = true

	instance, err := model.InstantiateWithMaterial(world, start, finish)
	if err != nil {
		t.Fatal(err)
	}

	parts := instance.Entities()
	if len(parts) != 2 {
		t.Fatalf("instantiated %d parts", len(parts))
	}

	for _, entity := range parts {
		shading, ok := engine.Get[material.SourceMaterial](world, entity)
		if !ok || !shading.HalfLambert {
			t.Fatal("part is missing the Source override")
		}

		if _, ok := engine.Get[material.Material](world, entity); ok {
			t.Fatal("part retained an authored Standard component")
		}
	}

	want := engine.Transform{Position: engine.Vector3{X: 3}, Scale: engine.Vector3{X: 2, Y: 2, Z: 2}}
	if err := instance.SetTransform(want); err != nil {
		t.Fatal(err)
	}

	invalid := want
	invalid.Scale.X = -1

	if err := instance.SetTransform(invalid); err == nil {
		t.Fatal("accepted a reflected root transform")
	}

	for _, entity := range parts {
		if got := world.Transform(entity); got == nil || *got != want {
			t.Fatalf("part did not retain the valid root transform: %v", got)
		}
	}

	world.Remove(parts[0])

	want.Position.Z = 4
	if err := instance.SetTransform(want); err != nil || *world.Transform(parts[1]) != want {
		t.Fatalf("partial removal prevented movement of a live part: %v", err)
	}

	instance.Remove()

	if err := instance.SetTransform(want); !errors.Is(err, ErrClosed) {
		t.Fatalf("removed instance accepted a transform: %v", err)
	}
}
