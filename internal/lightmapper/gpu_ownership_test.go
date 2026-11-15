//go:build !js

package lightmapper

import (
	"context"
	"reflect"
	"testing"
)

// Compare different tile shapes rather than depending on the scratch layout.
// Three owners and three probes exercise non-power-of-two state planes.
func gpuPathOwnershipQualification(t *testing.T, ctx context.Context, worker *GPUWorker) {
	t.Helper()

	scene := gpuFixtureScene(t, Scene{
		Environment: Environment{Sky: Vec3{.2, .15, .1}, Ground: Vec3{.1, .08, .05}},
		Emitters:    []Emitter{{ID: 91, Kind: Point, Position: Vec3{1, 2, 1}, Intensity: Vec3{2, 1, .5}}},
	})
	receivers := []Receiver{gpuFixtureReceiver(901, -1), gpuFixtureReceiver(17, 0), gpuFixtureReceiver(50, 1)}
	options := Options{Samples: 32, Bounces: 2, Outputs: OutputRGB | OutputRNM, Seed: 29}

	batch, err := worker.Bake(ctx, scene, receivers, options)
	if err != nil {
		t.Fatal(err)
	}

	probes := [][]Vec3{
		{{0, 0, 1}, {.6, 0, .8}, {0, .6, .8}},
		{{0, 0, 1}, {.6, 0, .8}, {0, .6, .8}},
		{{0, 0, 1}, {.6, 0, .8}, {0, .6, .8}},
	}

	responses, err := worker.BakeResponses(ctx, scene, receivers, options, probes)
	if err != nil {
		t.Fatal(err)
	}

	for index := range receivers {
		single, bakeErr := worker.Bake(ctx, scene, receivers[index:index+1], options)
		if bakeErr != nil || single.Estimates[0] != batch.Estimates[index] {
			t.Fatal("tile shape changed owned statistics", index, bakeErr)
		}

		response, verifyErr := worker.BakeResponses(ctx, scene, receivers[index:index+1], options, probes[index:index+1])
		if verifyErr != nil || !reflect.DeepEqual(response[0], responses[index]) {
			t.Fatal("tile shape changed owned directional statistics", index, verifyErr)
		}
	}

	t.Log("three path owners and three directional probes retain exact statistics across tile shapes")
}
