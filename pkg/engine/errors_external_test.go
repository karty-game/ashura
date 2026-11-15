package engine_test

import (
	"errors"
	"testing"

	"github.com/karty-game/karty-webgpu/pkg/compute"
	"github.com/karty-game/karty-webgpu/pkg/engine"
	"github.com/karty-game/karty-webgpu/pkg/shader"
	"github.com/karty-game/karty-webgpu/pkg/texture"
)

func TestPublicResourceErrorCategories(t *testing.T) {
	t.Parallel()

	app := engine.NewApp(engine.AppConfig{})
	defer func() { _ = app.Close() }()

	if err := app.UnloadTexture(nil); !errors.Is(err, engine.ErrOwner) {
		t.Fatalf("foreign texture error = %v", err)
	}

	if err := app.UnloadBuffer(nil); !errors.Is(err, engine.ErrOwner) {
		t.Fatalf("foreign buffer error = %v", err)
	}

	var recorder *engine.Frame
	if err := recorder.Dispatch(nil, nil, 1, 1, 1); !errors.Is(err, engine.ErrPhase) {
		t.Fatalf("dispatch phase error = %v", err)
	}

	if err := app.Close(); err != nil {
		t.Fatal(err)
	}

	if _, err := app.NewTexture(texture.Data{}); !errors.Is(err, engine.ErrClosed) {
		t.Fatalf("closed texture creation error = %v", err)
	}

	if _, err := shader.NewUniformBytes(app, make([]byte, 16)); !errors.Is(err, engine.ErrClosed) {
		t.Fatalf("closed buffer creation error = %v", err)
	}
}

func TestPublicComputeInUseErrorCategory(t *testing.T) {
	t.Parallel()

	app := engine.NewApp(engine.AppConfig{})
	defer func() { _ = app.Close() }()

	buffer, err := shader.NewUniformBytes(app, make([]byte, 16))
	if err != nil {
		t.Fatal(err)
	}

	pipeline, err := compute.NewPipeline(app, compute.PipelineDesc{
		Program:    shader.WGSL("fixture.wgsl", "@compute @workgroup_size(1) fn main() {}"),
		EntryPoint: "main", Layout: shader.Layout{shader.UniformLayout(0, 16)},
	})
	if err != nil {
		t.Fatal(err)
	}

	group, err := pipeline.BindGroup(shader.BindBuffer(0, buffer))
	if err != nil {
		t.Fatal(err)
	}

	if err := app.UnloadBuffer(buffer); !errors.Is(err, engine.ErrInUse) {
		t.Fatalf("bound buffer error = %v", err)
	}

	if err := app.UnloadComputePipeline(pipeline); !errors.Is(err, engine.ErrInUse) {
		t.Fatalf("bound pipeline error = %v", err)
	}

	if err := app.UnloadComputeBindings(group); err != nil {
		t.Fatal(err)
	}

	if err := app.UnloadBuffer(buffer); err != nil {
		t.Fatal(err)
	}

	if err := app.UnloadComputePipeline(pipeline); err != nil {
		t.Fatal(err)
	}
}

func TestWorldSetRejectsNilComponentWithoutMutation(t *testing.T) {
	t.Parallel()

	world := engine.NewWorld()
	entity := world.Add(engine.DefaultTransform())

	var missing *engine.Transform
	for _, value := range []any{nil, missing, &missing} {
		if err := world.Set(entity, value); !errors.Is(err, engine.ErrInvalidComponent) {
			t.Fatalf("Set(%T) error = %v", value, err)
		}
	}

	if world.Transform(entity) == nil || world.Transform(entity).Scale != engine.Vector3One() {
		t.Fatal("invalid component assignment changed existing component")
	}

	world.Remove(entity)

	if err := world.Set(entity, engine.Transform{}); !errors.Is(err, engine.ErrInvalidEntity) {
		t.Fatalf("removed entity error = %v", err)
	}
}
