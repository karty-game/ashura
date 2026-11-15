package lightmapper

import (
	"context"
	"math"
	"testing"
)

func testPointReceiver() Receiver {
	patch := ReceiverPatch{
		Positions:       [3]Vec3{{-1e-5, -1e-5, 0}, {1e-5, -1e-5, 0}, {0, 1e-5, 0}},
		Normals:         [3]Vec3{{0, 0, 1}, {0, 0, 1}, {0, 0, 1}},
		Area:            1,
		GeometricNormal: Vec3{0, 0, 1},
		Tangent:         Vec3{1, 0, 0},
		Handedness:      1,
	}

	return Receiver{ID: 1, Side: 1, Coverage: 1, Patches: []ReceiverPatch{patch}}
}

func TestReferencePointUnitsAndRGBRNMFlatParity(t *testing.T) {
	t.Parallel()

	scene := testCoreScene()
	scene.MetresPerUnit = 2
	scene.Emitters = []Emitter{{ID: 1, Kind: Point, Position: Vec3{0, 0, 2}, Intensity: Vec3{4, 2, 1}}}

	prepared, err := PrepareScene(scene)
	if err != nil {
		t.Fatal(err)
	}

	result, err := BakeReference(
		t.Context(),
		prepared,
		[]Receiver{testPointReceiver()},
		Options{Samples: 16, Outputs: OutputRGB | OutputRNM},
	)
	if err != nil {
		t.Fatal(err)
	}

	estimate := result.Estimates[0]

	for channel, light := range (Vec3{4, 2, 1}) {
		want := light / 16 / lightPi
		if math.Abs(float64(estimate.Statistics[0][channel]-want)) > 1e-6 {
			t.Fatal(estimate, want)
		}

		flat := (estimate.RNM[0][channel] + estimate.RNM[1][channel] + estimate.RNM[2][channel]) / 3
		if math.Abs(float64(flat-want)) > 1e-6 {
			t.Fatal("flat energy", flat, want)
		}
	}
}

func TestReferenceLegacyRangeFunction(t *testing.T) {
	t.Parallel()

	scene := testCoreScene()
	scene.MetresPerUnit = 99
	scene.Emitters = []Emitter{{ID: 1, Kind: Point, Model: LegacyRangeV1, Position: Vec3{0, 0, 2}, Intensity: Vec3{2, 2, 2}, Range: 10}}

	prepared, err := PrepareScene(scene)
	if err != nil {
		t.Fatal(err)
	}

	result, err := BakeReference(t.Context(), prepared, []Receiver{testPointReceiver()}, Options{Samples: 16})
	if err != nil {
		t.Fatal(err)
	}

	want := float32(2 * .96 * .96 / math.Pi)
	if math.Abs(float64(result.Estimates[0].Statistics[0][0]-want)) > 1e-6 {
		t.Fatal(result.Estimates[0], want)
	}
}

func TestReferenceAreaAndEnvironmentMIS(t *testing.T) {
	t.Parallel()

	for _, kind := range []string{"disk", "environment"} {
		t.Run(kind, func(t *testing.T) {
			t.Parallel()

			scene := testCoreScene()
			want := float32(.2)

			if kind == "disk" {
				scene.Emitters = []Emitter{
					{ID: 1, Kind: Disk, Position: Vec3{0, 0, 2}, Direction: Vec3{0, 0, -1}, Intensity: Vec3{1, 1, 1}, Radius: 1},
				}
			} else {
				scene.Environment = Environment{Sky: Vec3{.2, .2, .2}, Ground: Vec3{.2, .2, .2}}
			}

			prepared, err := PrepareScene(scene)
			if err != nil {
				t.Fatal(err)
			}

			result, err := BakeReference(t.Context(), prepared, []Receiver{testPointReceiver()}, Options{Samples: 4096, Seed: 29})
			if err != nil {
				t.Fatal(err)
			}

			estimate := result.Estimates[0]
			if math.Abs(float64(estimate.Statistics[0][0]-want)) > .006 {
				t.Fatal(kind, estimate.Statistics[0], want)
			}

			if estimate.Reflected != (Vec3{}) {
				t.Fatal("zero bounce reflected", estimate.Reflected)
			}
		})
	}
}

func TestReferenceRealDiffuseBounceAndCancellation(t *testing.T) {
	t.Parallel()

	scene := testCoreScene()
	scene.Materials = []Material{{Reflectance: Vec3{.8, .8, .8}}, {Reflectance: Vec3{.8, 0, 0}}}
	scene.Triangles = []Triangle{
		{ID: 1, Positions: [3]Vec3{{-100, -100, 0}, {100, -100, 0}, {0, 100, 0}}, Material: 0},
		{ID: 2, Positions: [3]Vec3{{-100, -100, 1}, {0, 100, 1}, {100, -100, 1}}, Material: 1},
	}
	scene.Emitters = []Emitter{{ID: 3, Kind: Point, Position: Vec3{0, 0, .5}, Intensity: Vec3{1, 1, 1}}}

	prepared, err := PrepareScene(scene)
	if err != nil {
		t.Fatal(err)
	}

	var values [3]Estimate

	for index, bounces := range []uint32{0, 1, 4} {
		result, err := BakeReference(
			t.Context(),
			prepared,
			[]Receiver{testPointReceiver()},
			Options{Samples: 1024, Seed: 17, Bounces: bounces},
		)
		if err != nil {
			t.Fatal(err)
		}

		values[index] = result.Estimates[0]
	}

	if values[1].Reflected[0] < .01 || values[2].Reflected[0] <= values[1].Reflected[0]+.001 {
		t.Fatal("missing bounce", values)
	}

	for _, value := range values {
		if value.Reflected[1] > 1e-6 || value.Reflected[2] > 1e-6 {
			t.Fatal("red bounce channel contamination", value)
		}
	}

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	if _, err := BakeReference(ctx, prepared, []Receiver{testPointReceiver()}, Options{Samples: 4}); err == nil {
		t.Fatal("cancellation ignored")
	}
}

func TestContinuationEmissionUsesRangeAndIndependentEmissionSidedness(t *testing.T) {
	t.Parallel()

	scene := testCoreScene()
	scene.Emitters = []Emitter{
		{ID: 1, Kind: Rectangle, Position: Vec3{0, 0, 2}, Direction: Vec3{0, 0, -1}, Intensity: Vec3{1, 1, 1}, Size: Vec2{2, 2}, Range: 1},
	}

	prepared, err := PrepareScene(scene)
	if err != nil {
		t.Fatal(err)
	}

	value := prepared.traceReference(Vec3{}, Vec3{0, 0, 1}, Vec3{0, 0, 1}, .5, 1, 0, Options{})
	if value.direct != (Vec3{}) {
		t.Fatal("finite support lost on emitter hit", value.direct)
	}

	scene.Emitters = []Emitter{{ID: 2, Kind: EmissiveTriangle, Triangle: 0, Intensity: Vec3{1, 1, 1}, TwoSided: true}}
	scene.Materials = []Material{{Reflectance: Vec3{.5, .5, .5}}}
	scene.Triangles = []Triangle{{ID: 1, Positions: [3]Vec3{{-2, -2, 2}, {2, -2, 2}, {0, 2, 2}}, Material: 0}}

	prepared, err = PrepareScene(scene)
	if err != nil {
		t.Fatal(err)
	}

	value = prepared.traceReference(Vec3{}, Vec3{0, 0, 1}, Vec3{0, 0, 1}, .5, 1, 0, Options{})
	if value.direct[0] <= 0 {
		t.Fatal("two-sided emission was hidden by one-sided reflection")
	}
}

func TestReferenceWorldUnitRescalingPreservesTransport(t *testing.T) {
	t.Parallel()

	var baseline Vec3

	for index, scale := range []float32{1, 1e-7, 1e6} {
		scene := testCoreScene()
		scene.MetresPerUnit = 1 / scale
		scene.Materials = []Material{{Reflectance: Vec3{.8, .8, .8}}}

		scene.Triangles = []Triangle{{ID: 1, Positions: [3]Vec3{{-100, -100, 1}, {0, 100, 1}, {100, -100, 1}}, Material: 0}}
		for j := range scene.Triangles[0].Positions {
			scene.Triangles[0].Positions[j] = Scale(scene.Triangles[0].Positions[j], scale)
		}

		scene.Emitters = []Emitter{{ID: 2, Kind: Point, Position: Vec3{0, 0, .5 * scale}, Intensity: Vec3{1, 1, 1}}}

		receiver := testPointReceiver()
		for j := range receiver.Patches[0].Positions {
			receiver.Patches[0].Positions[j] = Scale(receiver.Patches[0].Positions[j], scale)
		}

		prepared, err := PrepareScene(scene)
		if err != nil {
			t.Fatal(scale, err)
		}

		result, err := BakeReference(t.Context(), prepared, []Receiver{receiver}, Options{Samples: 256, Bounces: 1, Seed: 29})
		if err != nil {
			t.Fatal(scale, err)
		}

		value := result.Estimates[0].Statistics[0]
		if index == 0 {
			baseline = value
		} else {
			for channel := range 3 {
				if math.Abs(float64(value[channel]-baseline[channel])) > float64(baseline[channel])*.001 {
					t.Fatal("unit-dependent transport", scale, value, baseline)
				}
			}
		}
	}
}

func TestReferenceFullResponsesSharePathsAndCalibrateFlat(t *testing.T) {
	t.Parallel()

	scene := testCoreScene()
	scene.Emitters = []Emitter{{ID: 1, Kind: Point, Position: Vec3{0, 0, 2}, Intensity: Vec3{4, 4, 4}}}

	prepared, err := PrepareScene(scene)
	if err != nil {
		t.Fatal(err)
	}

	normals := make([]Vec3, 0, len(CalibrationProbes()))
	for _, probe := range CalibrationProbes() {
		normals = append(normals, probe.Normal)
	}

	result, err := ReferenceResponses(
		t.Context(),
		prepared,
		[]Receiver{testPointReceiver()},
		Options{Samples: 64, Seed: 17},
		[][]Vec3{normals},
	)
	if err != nil {
		t.Fatal(err)
	}

	for index, n := range normals {
		want := n[2] / lightPi

		got := result[0].Means[index][0]
		if math.Abs(float64(got-want)) > 1e-5 {
			t.Fatal(index, got, want)
		}
	}
}

func TestReferenceDiffuseBounceEvaluatesHitUVTexture(t *testing.T) {
	t.Parallel()

	scene := testCoreScene()
	scene.Materials = []Material{{Reflectance: Vec3{1, 1, 1}, Texture: 1}}
	scene.Textures = []Texture{{Width: 2, Height: 1, Pixels: []Vec3{{1, 0, 0}, {0, 0, 1}}}}
	scene.Triangles = []Triangle{
		{ID: 1, Material: 0, Positions: [3]Vec3{{-1, -1, 1}, {0, 1, 1}, {1, -1, 1}}, UV: [3]Vec2{{0, 0}, {.5, 1}, {1, 0}}},
	}
	scene.Emitters = []Emitter{{ID: 2, Kind: Point, Position: Vec3{0, 0, .5}, Intensity: Vec3{1, 1, 1}}}

	prepared, err := PrepareScene(scene)
	if err != nil {
		t.Fatal(err)
	}

	for index, positionX := range []float32{-.5, .5} {
		value := prepared.traceReference(Vec3{positionX, -.5, 0}, Vec3{0, 0, 1}, Vec3{0, 0, 1}, .5, 1, 0, Options{Bounces: 1})

		wanted := 0
		if index == 1 {
			wanted = 2
		}

		if value.reflected[wanted] <= 0 || value.reflected[1] != 0 || value.reflected[2-wanted] > 1e-6 {
			t.Fatal("bounce used mean or wrong UV texel", positionX, value.reflected)
		}
	}
}

func TestReferenceNarrowSunPreservesPhysicalEnergy(t *testing.T) {
	t.Parallel()

	scene := testCoreScene()
	scene.Emitters = []Emitter{{ID: 1, Kind: Sun, Direction: Vec3{0, 0, -1}, Radius: 1e-4, Intensity: Vec3{1e8, 1e8, 1e8}}}

	prepared, err := PrepareScene(scene)
	if err != nil {
		t.Fatal(err)
	}

	result, err := BakeReference(t.Context(), prepared, []Receiver{testPointReceiver()}, Options{Samples: 64})
	if err != nil {
		t.Fatal(err)
	}

	want := float32(1e8 * math.Pow(math.Sin(1e-4), 2))
	if got := result.Estimates[0].Statistics[0][0]; math.Abs(float64(got-want)) > 1e-5 {
		t.Fatal("narrow finite sun lost energy", got, want)
	}

	for _, u := range []float32{.01, .5, .99} {
		light := prepared.SampleEmitter(Vec3{}, 0, u, .37)

		pdf := prepared.EmitterPDF(Vec3{}, 0, light.Direction, light.Distance, Vec3{})
		if !finite(light.PDF) || math.Abs(float64(pdf-light.PDF)) > float64(pdf)*.0001 {
			t.Fatal(light, pdf)
		}
	}
}
