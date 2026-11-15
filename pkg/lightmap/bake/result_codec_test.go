//go:build !js

package lightmapbake

import (
	"reflect"
	"strings"
	"testing"

	"github.com/karty-game/karty-webgpu/internal/lightmapper"
)

func TestLinearArtifactRetainsHDRStatisticsAndRejectsCorruption(t *testing.T) {
	t.Parallel()

	result := Result{
		Schema:       lightmapper.ResultSchema,
		Algorithm:    lightmapper.Algorithm,
		SceneHash:    strings.Repeat("a", 64),
		ReceiverHash: strings.Repeat("b", 64),
		Options:      Options{Outputs: OutputRGB | OutputRNM, Samples: 16},
		Backend:      "fixture",
		Estimates: []Estimate{
			{
				ReceiverID:    1,
				Count:         16,
				Statistics:    [4]Vec3{{.0000001, 4, 4096}},
				SecondMoments: [4]Vec3{{.00000001, 16, 16777216}},
				RNM:           [3]Vec3{{.1, .2, .3}},
			},
		},
	}

	data, err := EncodeResult(result)
	if err != nil {
		t.Fatal(err)
	}

	got, err := DecodeResult(data)
	if err != nil || !reflect.DeepEqual(got, result) {
		t.Fatal(got, err)
	}

	for _, at := range []int{0, 8, 16, len(data) - 1} {
		corrupt := append([]byte(nil), data...)

		corrupt[at] ^= 1
		if _, err = DecodeResult(corrupt); err == nil {
			t.Fatal("corrupt result admitted", at)
		}
	}

	result.Estimates = append(result.Estimates, result.Estimates[0])

	data, err = EncodeResult(result)
	if err == nil {
		if _, err = DecodeResult(data); err == nil {
			t.Fatal("duplicate receiver accepted")
		}
	}
}

// Allocation measurements must remain serial on Go 1.27.
//
//nolint:paralleltest // Go 1.27 prohibits AllocsPerRun inside parallel tests.
func TestLinearArtifactAllocationsDoNotGrowPerReceiver(t *testing.T) {
	allocations := func(count int) (float64, float64) {
		result := Result{
			Schema: lightmapper.ResultSchema, Algorithm: lightmapper.Algorithm,
			SceneHash: strings.Repeat("a", 64), ReceiverHash: strings.Repeat("b", 64),
			Options: Options{Outputs: OutputRGB | OutputRNM, Samples: 1}, Backend: "fixture",
			Estimates: make([]Estimate, count),
		}
		for index := range result.Estimates {
			result.Estimates[index] = Estimate{ReceiverID: uint64(index + 1), Count: 1}
		}

		data, err := EncodeResult(result)
		if err != nil {
			t.Fatal(err)
		}

		encode := testing.AllocsPerRun(3, func() {
			if _, encodeErr := EncodeResult(result); encodeErr != nil {
				t.Fatal(encodeErr)
			}
		})
		decode := testing.AllocsPerRun(3, func() {
			if _, decodeErr := DecodeResult(data); decodeErr != nil {
				t.Fatal(decodeErr)
			}
		})

		return encode, decode
	}

	smallEncode, smallDecode := allocations(32)
	largeEncode, largeDecode := allocations(1024)
	// Result storage and the receiver-ID table may grow, but serializing another
	// 992 receivers must not create a temporary vector allocation for each one.
	if largeEncode > smallEncode+32 || largeDecode > smallDecode+64 {
		t.Fatal("per-receiver codec allocations", smallEncode, largeEncode, smallDecode, largeDecode)
	}
}
