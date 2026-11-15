//go:build !js

package model

import (
	"fmt"
	"math"
	"testing"

	"github.com/karty-game/karty-webgpu/internal/modeldata"
	"github.com/karty-game/karty-webgpu/pkg/engine"
)

func TestInstantiateRangeSelectsPreparedPartsAndPreservesLifetime(t *testing.T) {
	t.Parallel()
	_, data := fixtureAsset(t, false)

	document, err := modeldata.Decode(data)
	if err != nil {
		t.Fatal(err)
	}

	original := document.Parts[0]
	document.Parts = nil

	for index := range 3 {
		part := original
		part.Name = fmt.Sprintf("part%d", index)
		document.Parts = append(document.Parts, part)
	}

	data, err = modeldata.Encode(document)
	if err != nil {
		t.Fatal(err)
	}

	app := engine.NewApp(engine.AppConfig{})
	defer func() { _ = app.Close() }()

	loaded, err := Prepared("ranges", data).Load(app)
	if err != nil {
		t.Fatal(err)
	}

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
