//go:build !js

package level

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	compiler "github.com/karty-game/karty-webgpu/internal/modelbuild"
	"github.com/karty-game/karty-webgpu/internal/modelbuild/testfixture"
	"github.com/karty-game/karty-webgpu/pkg/engine"
	"github.com/karty-game/karty-webgpu/pkg/model"
	"github.com/karty-game/karty-webgpu/pkg/scene/resource"
)

func TestIndependentArrivalSharesLoadedModelAndRetainsEveryObject(t *testing.T) {
	t.Parallel()

	document, geometry := testfixture.Triangle()

	compiled, err := compiler.Compile(testfixture.GLB(document, geometry), "fixture.glb")
	if err != nil {
		t.Fatal(err)
	}

	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "fixture.kmodel"), compiled.Data, 0o600); err != nil {
		t.Fatal(err)
	}

	asset := resource.Asset{
		ID:     "any-model",
		Path:   "fixture.kmodel",
		Size:   len(compiled.Data),
		SHA256: fmt.Sprintf("%x", sha256.Sum256(compiled.Data)),
	}

	payload, err := resource.ReadVerified(t.Context(), root, asset)
	if err != nil {
		t.Fatal(err)
	}

	app := engine.NewApp(engine.AppConfig{})
	defer func() { _ = app.Close() }()

	world := engine.NewWorld()

	state, err := Open(t.Context(), app, world, OpenOptions{Base: root, Select: WholeScene})
	if err != nil {
		t.Fatal(err)
	}

	defer func() {
		err := state.Close()
		if err != nil {
			t.Error(err)
		}
	}()

	identity := [16]float32{1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1}
	reference := Reference{Path: asset.Path, Size: asset.Size, SHA256: asset.SHA256}

	groups, err := independentGroups(&Manifest{Objects: []Object{
		{ID: "/one", Model: asset.ID, Asset: &reference, Dynamic: true, Transform: identity},
		{ID: "/two", Model: asset.ID, Asset: &reference, Dynamic: true, Transform: identity},
	}}, []int{0, 1})
	if err != nil || len(groups) != 1 {
		t.Fatal("invalid independent fixture grouping", groups, err)
	}

	state.independent <- independentArrival{payload: payload, group: groups[0]}

	if err := state.pollIndependent(); err != nil {
		t.Fatal(err)
	}

	if len(state.models) != 1 || len(state.modelInstances) != 2 || world.Len() != 2 ||
		state.modelInstances[0].objectID != "/one" || state.modelInstances[1].objectID != "/two" {
		t.Fatal("shared resource duplicated or object placement lost", len(state.models), state.modelInstances, world.Len())
	}

	if err := app.Close(); err != nil || world.Len() != 0 || !state.Status().Closed {
		t.Fatal("independent instances escaped scene cleanup", err, world.Len())
	}
}

func TestPresentationCloseRetainsFailedOwnershipForRetry(t *testing.T) {
	t.Parallel()

	document, geometry := testfixture.Triangle()

	compiled, err := compiler.Compile(testfixture.GLB(document, geometry), "held.glb")
	if err != nil {
		t.Fatal(err)
	}

	app := engine.NewApp(engine.AppConfig{})
	defer func() { _ = app.Close() }()

	world := engine.NewWorld()

	presentation, err := Open(t.Context(), app, world, OpenOptions{Base: t.TempDir(), Select: WholeScene})
	if err != nil {
		t.Fatal(err)
	}

	asset, err := model.Prepared("held", compiled.Data).Load(app)
	if err != nil {
		t.Fatal(err)
	}

	presentation.models = append(presentation.models, asset)

	external, err := asset.Instantiate(world, engine.DefaultTransform())
	if err != nil {
		t.Fatal(err)
	}

	if err := presentation.Close(); !errors.Is(err, ErrInUse) || !presentation.Status().Closed || len(presentation.models) != 1 {
		t.Fatal("failed cleanup dropped retained resource ownership", err)
	}

	external.Remove()

	if err := presentation.Close(); err != nil || !presentation.released || len(presentation.models) != 0 {
		t.Fatal("resource release could not be retried", err)
	}
}
