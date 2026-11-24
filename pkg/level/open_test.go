package level

import (
	"context"
	"errors"
	"testing"

	"github.com/karty-game/ashura/pkg/engine"
)

func TestIndependentSelectionUsesPublishedOwnershipAndSharesExactResources(t *testing.T) {
	t.Parallel()

	identity := [16]float32{1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1}
	reference := Reference{Path: "levels/models/derived.kmodel", Size: 1, SHA256: "derived"}
	first := Object{ID: "/first", Model: "source#first", Dynamic: true, Asset: &reference, Transform: identity}
	second := first
	second.ID, second.Model = "/second", "another-source"
	alias := reference
	alias.Path = "levels/models/alias.kmodel"
	second.Asset = &alias
	otherReference := Reference{Path: "levels/models/other.kmodel", Size: 1, SHA256: "other"}
	third := first
	third.ID, third.Model, third.Asset = "/third", "third-source", &otherReference
	manifest := Manifest{Objects: []Object{{Model: "static"}, first, second, third}}

	groups, err := independentGroups(&manifest, []int{1, 2, 3})
	if err != nil || len(groups) != 2 || len(groups[0].objects) != 2 || len(groups[1].objects) != 1 ||
		groups[0].asset.Path != reference.Path || groups[0].objects[1].ID != second.ID {
		t.Fatal("independent resource grouping lost object ownership", groups, err)
	}

	conflict := second

	conflict.Model, conflict.Asset = first.Model, &otherReference
	if _, err := independentGroups(&Manifest{Objects: []Object{first, conflict}}, []int{0, 1}); !errors.Is(err, errIndependentReference) {
		t.Fatal("conflicting model reference admitted", err)
	}

	first.Asset = nil
	if _, err := independentGroups(&Manifest{Objects: []Object{first}}, []int{0}); !errors.Is(err, errIndependentReference) {
		t.Fatal("missing independent model admitted", err)
	}

	first.Asset, first.Transform[12] = &reference, 1
	if _, err := independentGroups(&Manifest{Objects: []Object{first}}, []int{0}); !errors.Is(err, errIndependentTransform) {
		t.Fatal("unprepared placement was silently transformed or ignored", err)
	}

	if groups, err := independentGroups(&Manifest{Objects: []Object{{Model: "static"}}}, nil); err != nil || len(groups) != 0 {
		t.Fatal("static geometry was selected as an independent model", groups, err)
	}
}

func TestPresentationCloseCancelsAndJoinsReadsBeforeRelease(t *testing.T) {
	t.Parallel()

	app := engine.NewApp(engine.AppConfig{})
	defer func() { _ = app.Close() }()

	ctx, cancel := context.WithCancel(t.Context())
	state := &Presentation{ctx: ctx, cancel: cancel, loader: NewLoader(app)}
	joined := false

	state.reads.Go(func() {
		<-ctx.Done()

		joined = true
	})

	if err := state.Close(); err != nil || !joined {
		t.Fatal("I/O lifetime escaped presentation cleanup", err, joined)
	}

	if err := state.Close(); err != nil || !state.Status().Closed {
		t.Fatal("repeated presentation cleanup failed", err)
	}

	if err := state.Update(0); err != nil {
		t.Fatal("closed sample update touched released resources", err)
	}
}
