//go:build !js

package reflectionbuild

import (
	"bytes"
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/karty-game/karty-webgpu/pkg/assetbuild"
	"github.com/karty-game/karty-webgpu/pkg/reflection/scene"
)

func TestPanoramaSeesTheVisibleRoom(t *testing.T) {
	t.Parallel()

	authored := authoredRoom(t)
	objects := primitives(authored.Objects)
	origin := vectorOf(authored.ProbePosition)

	targets := []struct {
		name  string
		point vector
	}{
		{"polished stone floor", vector{4, 0, 4}},
		{"polished ceiling", vector{0, 6, 0}},
		{"back wall", vector{0, 3, -6}},
		{"front wall", vector{0, 3, 6}},
		{"left wall", vector{-6, 3, 0}},
		{"right wall", vector{6, 3, 0}},
		{"center block", vector{0, .7, 0}},
		{"blue orb", vector{-2.8, .85, -1.5}},
		{"red pillar", vector{3, 1.5, -2.2}},
	}
	for _, target := range targets {
		direction := target.point.add(origin.scale(-1)).unit()

		index, normal := nearest(objects, origin, direction)
		if index < 0 || objects[index].object.Name != target.name {
			t.Errorf("ray toward %s hit object %d", target.name, index)
		}

		if normal.dot(direction) >= 0 {
			t.Errorf("ray toward %s hit a back face", target.name)
		}
	}

	pixels, err := render(t.Context(), authored)
	if err != nil {
		t.Fatal(err)
	}

	for offset := 3; offset < len(pixels.Pix); offset += 4 {
		if pixels.Pix[offset] != 255 {
			t.Fatalf("panorama has an uncaptured pixel at byte %d", offset)
		}
	}
}

func TestAuthoredPropEditsChangeTheBake(t *testing.T) {
	t.Parallel()

	authored := authoredRoom(t)

	before, err := render(t.Context(), authored)
	if err != nil {
		t.Fatal(err)
	}

	for index := range authored.Objects {
		object := &authored.Objects[index]
		if object.Name == "red pillar" {
			object.Position = reflectionscene.Vector3{X: -4, Y: 1.5, Z: 2}
			object.Rotation = reflectionscene.Vector3{X: .15, Y: .4, Z: .1}
			object.Surface.Color = reflectionscene.Color{G: .216, A: 1}
		}
	}

	objects := primitives(authored.Objects)
	origin := vectorOf(authored.ProbePosition)
	direction := vector{-4, -1.5, 2}.unit()

	index, _ := nearest(objects, origin, direction)
	if index < 0 || objects[index].object.Name != "red pillar" {
		t.Fatal("moved and rotated pillar is absent from its new bake position")
	}

	previous, _ := nearest(objects, origin, vector{3, -1.5, -2.2}.unit())
	if previous >= 0 && objects[previous].object.Name == "red pillar" {
		t.Fatal("pillar remains at its old bake position")
	}

	after, err := render(t.Context(), authored)
	if err != nil {
		t.Fatal(err)
	}

	if bytes.Equal(before.Pix, after.Pix) {
		t.Fatal("authored prop edits left the panorama unchanged")
	}
}

func TestCanceledBakeStopsBeforeRendering(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	if _, err := render(ctx, authoredRoom(t)); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled render: %v", err)
	}
}

func authoredRoom(t *testing.T) reflectionscene.Scene {
	t.Helper()

	project, err := filepath.Abs("testdata")
	if err != nil {
		t.Fatal(err)
	}

	scene, _, err := loadSource(&assetbuild.Plan{Project: project}, fixtureOptions())
	if err != nil {
		t.Fatal(err)
	}

	return scene
}
