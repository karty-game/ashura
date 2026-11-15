package level

import (
	"context"
	"errors"
	"testing"

	"github.com/karty-game/karty-webgpu/pkg/engine"
)

var errSelectionFixture = errors.New("selection fixture rejected residency")

func TestOpenRequiresExplicitSelectionAndDefersDeviceAndIO(t *testing.T) {
	t.Parallel()

	app := engine.NewApp(engine.AppConfig{})
	defer func() { _ = app.Close() }()

	world := engine.NewWorld()

	options := OpenOptions{Base: t.TempDir(), Select: WholeScene}
	if _, err := Open(t.Context(), app, world, OpenOptions{Base: options.Base}); !errors.Is(err, ErrFormat) {
		t.Fatal("implicit world selection was admitted", err)
	}

	presentation, err := Open(t.Context(), app, world, options)
	if err != nil {
		t.Fatal("opening needs neither device nor source files", err)
	}

	if presentation.started || presentation.Status() != (PresentationStatus{}) {
		t.Fatal("Open started loading or device selection")
	}

	if err := app.Close(); err != nil || !presentation.Status().Closed {
		t.Fatal("App lifetime did not close its presentation", err)
	}
}

func TestOpenSelectionSnapshotsAndAdmissionBeforePayloadIO(t *testing.T) {
	t.Parallel()

	manifest := Manifest{Chunks: []Chunk{{Space: "one"}, {Space: "one"}}, Objects: []Object{{Dynamic: true}}}
	selection := Selection{Chunks: []int{1}, Objects: []int{0}}
	owned := cloneSelection(selection)

	selection.Chunks[0], selection.Objects[0] = -1, -1
	if owned.Chunks[0] != 1 || owned.Objects[0] != 0 {
		t.Fatal("caller changed an owned selection")
	}

	for _, invalid := range []Selection{
		{Chunks: []int{-1}}, {Chunks: []int{2}}, {Chunks: []int{0, 0}},
		{Objects: []int{-1}}, {Objects: []int{1}}, {Objects: []int{0, 0}},
	} {
		if err := validatePresentationSelection(manifest, invalid); !errors.Is(err, ErrFormat) {
			t.Fatal("invalid selection admitted", invalid, err)
		}
	}

	manifest.Objects[0].Dynamic = false
	if err := validatePresentationSelection(manifest, owned); !errors.Is(err, ErrFormat) {
		t.Fatal("static model selected as independent")
	}

	app := engine.NewApp(engine.AppConfig{})
	defer func() { _ = app.Close() }()

	presentation, err := Open(t.Context(), app, engine.NewWorld(), OpenOptions{
		Base: t.TempDir(), Select: func(Manifest) (Selection, error) { return Selection{}, errSelectionFixture },
	})
	if err != nil {
		t.Fatal(err)
	}

	if err := presentation.accept(
		presentationArrival{manifest: &manifest},
	); !errors.Is(err, errSelectionFixture) ||
		presentation.manifest != nil {
		t.Fatal("rejected policy started resident payload work", err)
	}
}

func TestWholeSceneDoesNotInferCrossSpaceReachability(t *testing.T) {
	t.Parallel()

	manifest := Manifest{Chunks: []Chunk{{Space: "first"}, {Space: "second"}}, Objects: []Object{{}, {Dynamic: true}}}
	if _, err := WholeScene(manifest); !errors.Is(err, ErrFormat) {
		t.Fatal("whole-scene convenience crossed logical spaces", err)
	}

	manifest.Chunks[1].Space = "first"

	selection, err := WholeScene(manifest)
	if err != nil || len(selection.Chunks) != 2 || len(selection.Objects) != 1 || selection.Objects[0] != 1 {
		t.Fatal("explicit single-space scene selection changed", selection, err)
	}
}

func TestOpenCanceledContextAndClosedUpdates(t *testing.T) {
	t.Parallel()

	app := engine.NewApp(engine.AppConfig{})
	defer func() { _ = app.Close() }()

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	if _, err := Open(ctx, app, engine.NewWorld(), OpenOptions{Base: ".", Select: WholeScene}); !errors.Is(err, context.Canceled) {
		t.Fatal("canceled opening admitted", err)
	}
}

func TestPresentationCancellationPrecedesDeferredDeviceSelection(t *testing.T) {
	t.Parallel()

	app := engine.NewApp(engine.AppConfig{})
	defer func() { _ = app.Close() }()

	ctx, cancel := context.WithCancel(t.Context())

	presentation, err := Open(ctx, app, engine.NewWorld(), OpenOptions{Base: t.TempDir(), Select: WholeScene})
	if err != nil {
		t.Fatal(err)
	}

	cancel()

	if err := presentation.Update(0); !errors.Is(err, context.Canceled) {
		t.Fatal("canceled loading attempted device selection", err)
	}

	if err := presentation.Close(); err != nil {
		t.Fatal(err)
	}
}
