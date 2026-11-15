package lightmapper

import (
	"fmt"
	"math"
	"testing"
)

//nolint:gocognit // Both spatial distributions compare identical exhaustive and sampled sums at every scale.
func TestSampledManyLightDiffuseAgainstExhaustive(t *testing.T) {
	t.Parallel()

	for _, count := range []int{1, 64, 1024, 16384} {
		for _, distributed := range []bool{false, true} {
			t.Run(fmt.Sprintf("lights%d/distributed%v", count, distributed), func(t *testing.T) {
				t.Parallel()

				source := testCoreScene()
				source.Emitters = make([]Emitter, count)
				rows := (count + 127) / 128

				columns := min(count, 128)
				for emitterIndex := range count {
					coordinateX, coordinateY := float32(
						emitterIndex%columns,
					)-float32(
						columns-1,
					)*.5, float32(
						emitterIndex/columns,
					)-float32(
						rows-1,
					)*.5
					model, radius := Physical, float32(0)
					spacing := float32(.01)

					if distributed {
						model = LegacyRangeV1
						radius = 6
						spacing = 2
					}

					weight := float32(1+(emitterIndex*13)%31) / float32(count)
					source.Emitters[emitterIndex] = Emitter{
						ID:        uint64(emitterIndex + 1),
						Kind:      Point,
						Model:     model,
						Position:  Vec3{coordinateX * spacing, coordinateY * spacing, 1.5 + float32(emitterIndex%7)*.05},
						Range:     radius,
						Intensity: Vec3{weight, weight * .6, weight * .2},
					}
				}

				prepared, err := PrepareScene(source)
				if err != nil {
					t.Fatal(err)
				}

				var expected [3]float64

				for emitterIndex := range count {
					sample := prepared.SampleEmitter(Vec3{}, uint32(emitterIndex), .5, .5)
					for c := range 3 {
						expected[c] += float64(sample.Radiance[c] * max(0, sample.Direction[2]) / lightPi)
					}
				}

				const samples = 65536

				var actual [3]float64

				for emitterIndex := range uint32(samples) {
					index, mass := prepared.SampleLight(Vec3{}, SampleDimension(1, 29, emitterIndex, 0, 3))

					sample := prepared.SampleEmitter(Vec3{}, index, .5, .5)
					for c := range 3 {
						actual[c] += float64(sample.Radiance[c]*max(0, sample.Direction[2])/(lightPi*mass)) / samples
					}
				}

				for c := range 3 {
					if math.Abs(actual[c]-expected[c]) > max(1e-8, expected[c]*.025) {
						t.Fatal("sampled/exhaustive many-light mismatch", actual, expected)
					}
				}

				t.Logf(
					"emitters=%d distributed=%v red-relative-error=%.5f",
					count,
					distributed,
					math.Abs(actual[0]-expected[0])/expected[0],
				)
			})
		}
	}
}
