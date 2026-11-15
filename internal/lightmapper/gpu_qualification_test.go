//go:build !js

package lightmapper

import (
	"context"
	"math"
	"testing"
)

func gpuQualification(t *testing.T, ctx context.Context, worker *GPUWorker) {
	t.Helper()
	gpuPathOwnershipQualification(t, ctx, worker)
	gpuGlobalLightQualification(t, ctx, worker)
	gpuBounceQualification(t, ctx, worker)
	gpuUnitsQualification(t, ctx, worker)
	gpuHDRQualification(t, ctx, worker)
	gpuPolarEnvironmentQualification(t, ctx, worker)
	gpuThinTriangleQualification(t, ctx, worker)
	gpuFootprintShadowQualification(t, ctx, worker)
}

func gpuBounceQualification(t *testing.T, ctx context.Context, worker *GPUWorker) {
	t.Helper()

	source := testCoreScene()
	source.Materials = []Material{{Reflectance: Vec3{.8, .8, .8}}, {Reflectance: Vec3{.8, 0, 0}}}
	source.Triangles = []Triangle{
		{ID: 1, Positions: [3]Vec3{{-100, -100, 0}, {100, -100, 0}, {0, 100, 0}}, Material: 0},
		{ID: 2, Positions: [3]Vec3{{-100, -100, 1}, {0, 100, 1}, {100, -100, 1}}, Material: 1},
	}
	source.Emitters = []Emitter{{ID: 3, Kind: Point, Position: Vec3{0, 0, .5}, Intensity: Vec3{1, 1, 1}}}

	scene, err := PrepareScene(source)
	if err != nil {
		t.Fatal(err)
	}

	var reflected [3]float32

	for index, bounces := range []uint32{0, 1, 4} {
		options := Options{Outputs: OutputRGB | OutputRNM, Samples: 256, Seed: 17, Bounces: bounces}
		receivers := []Receiver{testPointReceiver()}

		gpu, err := worker.Bake(ctx, scene, receivers, options)
		if err != nil {
			t.Fatal(err)
		}

		cpu, err := BakeReference(ctx, scene, receivers, options)
		if err != nil {
			t.Fatal(err)
		}

		gpuCompare(t, gpu, cpu, .003)

		reflected[index] = gpu.Estimates[0].Reflected[0]
		if gpu.Estimates[0].Reflected[1] > 1e-6 || gpu.Estimates[0].Reflected[2] > 1e-6 {
			t.Fatal("bounce reflectance color lost", gpu.Estimates[0].Reflected)
		}
	}

	if reflected[0] != 0 || reflected[1] < .01 || reflected[2] <= reflected[1]+.001 {
		t.Fatal("missing first/subsequent compute bounces", reflected)
	}

	t.Log("zero/one/four compute bounces", reflected)
}

func gpuUnitsQualification(t *testing.T, ctx context.Context, worker *GPUWorker) {
	t.Helper()

	var baseline Vec3

	for index, scale := range []float32{1, 1e-7, 1e6} {
		source := testCoreScene()
		source.MetresPerUnit = 1 / scale
		source.Materials = []Material{{Reflectance: Vec3{.8, .8, .8}}}

		source.Triangles = []Triangle{{ID: 1, Positions: [3]Vec3{{-100, -100, 1}, {0, 100, 1}, {100, -100, 1}}, Material: 0}}
		for j := range source.Triangles[0].Positions {
			source.Triangles[0].Positions[j] = Scale(source.Triangles[0].Positions[j], scale)
		}

		source.Emitters = []Emitter{{ID: 2, Kind: Point, Position: Vec3{0, 0, .5 * scale}, Intensity: Vec3{1, 1, 1}}}

		receiver := testPointReceiver()
		for j := range receiver.Patches[0].Positions {
			receiver.Patches[0].Positions[j] = Scale(receiver.Patches[0].Positions[j], scale)
		}

		scene, err := PrepareScene(source)
		if err != nil {
			t.Fatal(err)
		}

		options := Options{Outputs: OutputRGB | OutputRNM, Samples: 256, Bounces: 1, Seed: 29}

		result, err := worker.Bake(ctx, scene, []Receiver{receiver}, options)
		if err != nil {
			t.Fatal("world scale", scale, err)
		}

		value := result.Estimates[0].Statistics[0]
		if index == 0 {
			baseline = value
		} else {
			for c := range 3 {
				if math.Abs(float64(value[c]-baseline[c])) > float64(baseline[c])*.002 {
					t.Fatal("unit-dependent GPU transport", scale, value, baseline)
				}
			}
		}
	}

	t.Log("physical transport unchanged across world scales 1e-7 / 1 / 1e6")
}

func gpuHDRQualification(t *testing.T, ctx context.Context, worker *GPUWorker) {
	t.Helper()

	source := testCoreScene()
	source.Emitters = []Emitter{{ID: 1, Kind: Point, Position: Vec3{0, 0, 2}, Intensity: Vec3{1e22, 8e21, 5e21}}}

	scene, err := PrepareScene(source)
	if err != nil {
		t.Fatal(err)
	}

	receivers := []Receiver{testPointReceiver()}
	options := Options{Outputs: OutputRGB | OutputRNM, Samples: 32, Seed: 71}

	gpu, err := worker.Bake(ctx, scene, receivers, options)
	if err != nil {
		t.Fatal("finite HDR energy rejected", err)
	}

	cpu, err := BakeReference(ctx, scene, receivers, options)
	if err != nil {
		t.Fatal(err)
	}

	actual, expected := gpu.Estimates[0], cpu.Estimates[0]
	if !actual.VarianceValid || actual.SecondMomentsValid {
		t.Fatal("HDR statistics validity", actual.VarianceValid, actual.SecondMomentsValid)
	}

	for kernel := range 4 {
		for c := range 3 {
			if math.Abs(
				float64(actual.Statistics[kernel][c]-expected.Statistics[kernel][c]),
			) > math.Abs(
				float64(expected.Statistics[kernel][c]),
			)*.0005 {
				t.Fatal("HDR mean", actual.Statistics[kernel], expected.Statistics[kernel])
			}
		}
	}

	var varianceTotal float64

	for kernel := range 4 {
		for c := range 3 {
			v, reference := float64(actual.Variance[kernel][c]), float64(expected.Variance[kernel][c])

			varianceTotal += v
			if reference > 0 && math.Abs(v-reference) > reference*.15 {
				t.Fatal("HDR stable variance differs from CPU Welford", kernel, c, v, reference)
			}
		}
	}

	if varianceTotal == 0 {
		t.Fatal("bright directional variation disappeared through square cancellation")
	}

	for kernel := range 3 {
		for c := range 3 {
			if math.Abs(float64(actual.RNM[kernel][c]-expected.RNM[kernel][c])) > math.Abs(float64(expected.RNM[kernel][c]))*.001+1e14 {
				t.Fatal("HDR RNM fit", actual.RNM, expected.RNM)
			}
		}
	}

	t.Log("finite HDR means/projection survive optional raw-square overflow with stable Welford variance")
}

func gpuPolarEnvironmentQualification(t *testing.T, ctx context.Context, worker *GPUWorker) {
	t.Helper()

	texture := Texture{Width: 4, Height: 8192, Pixels: make([]Vec3, 4*8192)}
	for index := range texture.Pixels {
		texture.Pixels[index] = Vec3{.001, .001, .001}
	}

	texture.Pixels[0] = Vec3{1e8, 8e7, 5e7}
	scene := gpuFixtureScene(t, Scene{Textures: []Texture{texture}, Environment: Environment{Texture: 1}})
	options := Options{Outputs: OutputRGB | OutputRNM, Samples: 512, Seed: 139}
	receivers := []Receiver{gpuFixtureReceiver(503, 0)}

	gpu, err := worker.Bake(ctx, scene, receivers, options)
	if err != nil {
		t.Fatal("polar environment", err)
	}

	cpu, err := BakeReference(ctx, scene, receivers, options)
	if err != nil {
		t.Fatal(err)
	}

	for kernel := range 4 {
		for c := range 3 {
			actual, expected := gpu.Estimates[0].Statistics[kernel][c], cpu.Estimates[0].Statistics[kernel][c]
			if math.Abs(float64(actual-expected)) > math.Abs(float64(expected))*.015+.0001 {
				t.Fatal("polar environment PDF/radiance mismatch", kernel, c, actual, expected)
			}
		}
	}

	t.Log("8192-row HDR polar environment matches CPU solid-angle reference")
}

func gpuThinTriangleQualification(t *testing.T, ctx context.Context, worker *GPUWorker) {
	t.Helper()
	scene := gpuFixtureScene(
		t,
		Scene{
			Materials: []Material{{}},
			Triangles: []Triangle{{ID: 1, Positions: [3]Vec3{{0, 0, 1}, {1, 0, 1}, {1, 1e-9, 1}}}},
			Emitters:  []Emitter{{ID: 2, Kind: Point, Position: Vec3{.75, 2.5e-10, 2}, Intensity: Vec3{1, 1, 1}}},
		},
	)
	patch := ReceiverPatch{
		Area:            1,
		Positions:       [3]Vec3{{.749999, 2.49e-10, 0}, {.750001, 2.49e-10, 0}, {.75, 2.51e-10, 0}},
		Normals:         [3]Vec3{{0, 0, 1}, {0, 0, 1}, {0, 0, 1}},
		GeometricNormal: Vec3{0, 0, 1},
	}
	receiver := Receiver{ID: 703, Side: 1, Coverage: 1, Patches: []ReceiverPatch{patch}}

	result, err := worker.Bake(ctx, scene, []Receiver{receiver}, Options{Samples: 16, Outputs: OutputRGB | OutputRNM})
	if err != nil {
		t.Fatal(err)
	}

	if result.Estimates[0].Statistics[0] != (Vec3{}) {
		t.Fatal("nearly parallel triangle edges lost shadow visibility", result.Estimates[0].Statistics[0])
	}

	t.Log("thin triangle with nearly parallel edges retains geometric occlusion")
}

func gpuFootprintShadowQualification(t *testing.T, ctx context.Context, worker *GPUWorker) {
	t.Helper()

	source := Scene{
		Materials: []Material{{}},
		Triangles: []Triangle{{ID: 1, Positions: [3]Vec3{{0, 0, -10}, {0, 3, 0}, {0, 0, 10}}}},
		Emitters:  []Emitter{{ID: 2, Kind: Point, Position: Vec3{1, 2, 0}, Intensity: Vec3{16, 16, 16}}},
	}
	scene := gpuFixtureScene(t, source)
	receiver := Receiver{
		ID:       709,
		Side:     1,
		Coverage: 1,
		Patches: []ReceiverPatch{
			{
				Area:            1,
				Positions:       [3]Vec3{{-1, 0, -1}, {-1, 0, 1}, {1, 0, 0}},
				Normals:         [3]Vec3{{0, 1, 0}, {0, 1, 0}, {0, 1, 0}},
				GeometricNormal: Vec3{0, 1, 0},
			},
		},
	}
	options := Options{Samples: 512, Outputs: OutputRGB | OutputRNM, Seed: 13}

	sampled, err := worker.Bake(ctx, scene, []Receiver{receiver}, options)
	if err != nil {
		t.Fatal(err)
	}

	reference, err := BakeReference(ctx, scene, []Receiver{receiver}, options)
	if err != nil {
		t.Fatal(err)
	}

	gpuCompare(t, sampled, reference, .002)

	center := gpuFixtureReceiver(711, -1.0/3)

	dark, err := worker.Bake(ctx, scene, []Receiver{center}, options)
	if err != nil {
		t.Fatal(err)
	}

	if sampled.Estimates[0].Statistics[0][0] <= .01 || dark.Estimates[0].Statistics[0] != (Vec3{}) {
		t.Fatal(
			"footprint integration collapsed to center-sampled shadow",
			sampled.Estimates[0].Statistics[0],
			dark.Estimates[0].Statistics[0],
		)
	}

	t.Log("one luxel integrates visible and shadowed surface support while its center is dark")
}
