package engine_test

import (
	_ "embed"
	"testing"

	"github.com/karty-game/karty-webgpu/pkg/compute"
	"github.com/karty-game/karty-webgpu/pkg/engine"
	"github.com/karty-game/karty-webgpu/pkg/shader"
)

//go:embed wgsl/compute-fixture.wgsl
var ownershipWGSL string

func TestComputeResourceOwnershipAndUnload(t *testing.T) {
	t.Parallel()

	app := engine.NewApp(engine.AppConfig{})
	defer func() { _ = app.Close() }()

	foreign := engine.NewApp(engine.AppConfig{})
	defer func() { _ = foreign.Close() }()

	const uniformSize = 16

	params, err := shader.NewUniformBytes(app, make([]byte, uniformSize))
	if err != nil {
		t.Fatal(err)
	}

	foreignParams, err := shader.NewUniformBytes(foreign, make([]byte, uniformSize))
	if err != nil {
		t.Fatal(err)
	}

	const textureSize = 4

	image, err := compute.NewTexture(app, textureSize, textureSize)
	if err != nil {
		t.Fatal(err)
	}

	kernel, err := compute.NewPipeline(app, compute.PipelineDesc{
		Program: shader.WGSL("wgsl/compute-fixture.wgsl", ownershipWGSL), EntryPoint: "main",
		Layout: shader.Layout{shader.UniformLayout(0, uniformSize), shader.StorageTextureLayout(1)},
	})
	if err != nil {
		t.Fatal(err)
	}

	if _, err := kernel.BindGroup(shader.BindBuffer(0, foreignParams), shader.BindTexture(1, image)); err == nil {
		t.Fatal("accepted a buffer from a different App")
	}

	group, err := kernel.BindGroup(shader.BindBuffer(0, params), shader.BindTexture(1, image))
	if err != nil {
		t.Fatal(err)
	}

	var outside engine.Frame
	if err := outside.Dispatch(kernel, group, 1, 1, 1); err == nil {
		t.Fatal("dispatched outside an App compute callback")
	}

	if err := foreign.UnloadBuffer(params); err == nil {
		t.Fatal("foreign App unloaded a buffer")
	}

	if err := app.UnloadBuffer(params); err == nil {
		t.Fatal("unloaded a bound uniform")
	}

	if err := app.UnloadTexture(image); err == nil {
		t.Fatal("unloaded a bound storage texture")
	}

	if err := app.UnloadComputePipeline(kernel); err == nil {
		t.Fatal("unloaded a pipeline with live bindings")
	}

	if err := app.UnloadComputeBindings(group); err != nil {
		t.Fatal(err)
	}

	if err := app.UnloadBuffer(params); err != nil {
		t.Fatal(err)
	}

	if err := params.Write(0, make([]byte, uniformSize)); err == nil {
		t.Fatal("wrote to an unloaded buffer")
	}

	if _, err := kernel.BindGroup(shader.BindBuffer(0, params), shader.BindTexture(1, image)); err == nil {
		t.Fatal("bound an unloaded buffer")
	}

	if err := app.UnloadTexture(image); err != nil {
		t.Fatal(err)
	}

	if err := app.UnloadComputePipeline(kernel); err != nil {
		t.Fatal(err)
	}

	if _, err := kernel.BindGroup(); err == nil {
		t.Fatal("created bindings from an unloaded pipeline")
	}

	if err := foreign.Close(); err != nil {
		t.Fatal(err)
	}

	if err := foreignParams.Write(0, make([]byte, uniformSize)); err == nil {
		t.Fatal("App.Close did not invalidate its buffers")
	}

	if _, err := shader.NewUniformBytes(foreign, make([]byte, uniformSize)); err == nil {
		t.Fatal("created a buffer after App.Close")
	}
}
