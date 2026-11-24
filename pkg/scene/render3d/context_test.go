//go:build !js

package render3d_test

import (
	"errors"
	"testing"

	"github.com/karty-game/ashura/internal/modelbuild"
	"github.com/karty-game/ashura/internal/modelbuild/testfixture"
	"github.com/karty-game/ashura/pkg/engine"
	"github.com/karty-game/ashura/pkg/model"
	"github.com/karty-game/ashura/pkg/scene"
	scene3d "github.com/karty-game/ashura/pkg/scene/render3d"
)

func TestSupplyDistinguishesInvalidContextFromClosedLifetime(t *testing.T) {
	t.Parallel()

	var absent *scene3d.Context
	if err := absent.Supply("triangle", nil); !errors.Is(err, scene3d.ErrInvalid) {
		t.Fatalf("nil context must be invalid construction input: %v", err)
	}

	resources := scene3d.NewContext(engine.NewWorld())
	if err := resources.Close(); err != nil {
		t.Fatal(err)
	}

	if err := resources.Supply("triangle", nil); !errors.Is(err, scene.ErrClosed) {
		t.Fatalf("closed context must return its lifetime category: %v", err)
	}
}

func TestResourceArrivalEvictionAndVisibilityNeverResetLogicalState(t *testing.T) {
	t.Parallel()

	app := engine.NewApp(engine.AppConfig{})
	defer func() { _ = app.Close() }()

	world := engine.NewWorld()
	resources := scene3d.NewContext(world)
	root, _ := scene.NewRoot("world")

	state, err := scene.Bool(root, nil, "switch", false, false)
	if err != nil {
		t.Fatal(err)
	}

	view, err := scene3d.NewModel(root, resources, "triangle", engine.DefaultTransform())
	if err != nil || view.Resident() || world.Len() != 0 {
		t.Fatalf("model declaration loaded a resource: %v", err)
	}

	if err := state.Set(true); err != nil {
		t.Fatal(err)
	}

	document, data := testfixture.Triangle()

	prepared, err := modelbuild.Compile(testfixture.GLB(document, data), "triangle")
	if err != nil {
		t.Fatal(err)
	}

	load := func() {
		asset, err := model.Prepared("triangle", prepared.Data).Load(app)
		if err != nil {
			t.Fatal(err)
		}

		if err := resources.Supply("triangle", asset); err != nil {
			t.Fatal(err)
		}

		if err := resources.Supply("triangle", asset); !errors.Is(err, scene3d.ErrSupplied) {
			t.Fatalf("duplicate supply must return the public category without replacing ownership: %v", err)
		}
	}
	load()

	if !view.Resident() || world.Len() != 1 {
		t.Fatal("pending view did not attach")
	}

	if err := view.SetVisible(false); err != nil {
		t.Fatal(err)
	}

	if world.Len() != 0 || !state.Get() || root.Closed() {
		t.Fatal("visibility affected gameplay state")
	}

	if err := view.SetVisible(true); err != nil {
		t.Fatal(err)
	}

	if err := resources.Evict("triangle"); err != nil {
		t.Fatal(err)
	}

	if world.Len() != 0 || view.Resident() || !state.Get() || root.Closed() {
		t.Fatal("resource eviction affected gameplay state")
	}

	load()

	if !view.Resident() || !state.Get() {
		t.Fatal("resource reacquisition reset state")
	}

	root.Close()

	if world.Len() != 0 {
		t.Fatal("root close retained presentation")
	}

	if _, err := scene3d.NewModel(root, resources, "triangle", engine.DefaultTransform()); !errors.Is(err, scene.ErrClosed) {
		t.Fatalf("closed root must retain the logical scene error: %v", err)
	}

	if err := resources.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestBakedPlacementDoesNotDuplicateSourceModelWhenDynamicInstanceArrives(t *testing.T) {
	t.Parallel()

	app := engine.NewApp(engine.AppConfig{})
	defer func() { _ = app.Close() }()

	world := engine.NewWorld()
	resources := scene3d.NewContext(world)
	root, _ := scene.NewRoot("level")

	baked, err := scene3d.NewBakedModel(root, resources, "shared", engine.DefaultTransform())
	if err != nil {
		t.Fatal(err)
	}

	placement := engine.DefaultTransform()
	placement.Position = engine.Vector3{X: 12, Y: 2, Z: -3}

	dynamic, err := scene3d.NewModel(root, resources, "shared", placement)
	if err != nil {
		t.Fatal(err)
	}

	document, data := testfixture.Triangle()

	prepared, err := modelbuild.Compile(testfixture.GLB(document, data), "shared")
	if err != nil {
		t.Fatal(err)
	}

	asset, err := model.Prepared("shared", prepared.Data).Load(app)
	if err != nil {
		t.Fatal(err)
	}

	if err = resources.Supply("shared", asset); err != nil {
		t.Fatal(err)
	}

	if baked.Resident() || !dynamic.Resident() || world.Len() != 1 {
		t.Fatal("source geometry duplicated a placement already represented by static chunks")
	}

	world.Each(func(entity engine.Entity) {
		transform := world.Transform(entity)
		if transform == nil || transform.Position != placement.Position {
			t.Fatal("resource arrival lost the authored independent placement")
		}
	})

	if err = baked.SetVisible(false); err != nil {
		t.Fatal(err)
	}

	if err = baked.SetVisible(true); err != nil || baked.Resident() || world.Len() != 1 {
		t.Fatalf("logical visibility instantiated baked source geometry: %v", err)
	}

	if err = resources.Evict("shared"); err != nil || root.Closed() || baked.Resident() || dynamic.Resident() {
		t.Fatalf("eviction changed baked logical ownership: %v", err)
	}

	root.Close()

	if err = resources.Close(); err != nil || world.Len() != 0 {
		t.Fatalf("baked placeholder cleanup: %v", err)
	}
}
