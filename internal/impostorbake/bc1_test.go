//go:build !js

package impostorbake

import (
	"bytes"
	"context"
	"errors"
	"math"
	"testing"
)

func TestBC1NormalDepthPackingKeepsDepthContinuousAndSourceImmutable(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		pixel [4]byte
		oct   [2]byte
	}{
		{[4]byte{128, 128, 255, 7}, [2]byte{128, 128}},
		{[4]byte{255, 128, 128, 93}, [2]byte{254, 128}},
		{[4]byte{0, 128, 128, 172}, [2]byte{1, 128}},
		{[4]byte{128, 128, 0, 250}, [2]byte{255, 255}},
	} {
		pixels := append([]byte(nil), test.pixel[:]...)

		packed, err := PackBC1NormalDepth(t.Context(), pixels)
		if err != nil {
			t.Fatal(err)
		}

		if !bytes.Equal(pixels, test.pixel[:]) || len(packed) != channels || packed[1] != test.pixel[3] || packed[3] != 255 {
			t.Fatalf("packing changed source or lost continuous depth: source %v packed %v", pixels, packed)
		}

		if math.Abs(float64(packed[0])-float64(test.oct[0])) > 1 || math.Abs(float64(packed[2])-float64(test.oct[1])) > 1 {
			t.Fatalf("normal direction packed to %v, want R/B near %v", packed, test.oct)
		}

		packed[0] ^= 255

		if !bytes.Equal(pixels, test.pixel[:]) {
			t.Fatal("packed mip aliases its source")
		}
	}
}

func TestBC1NormalDepthPackingHonorsCancellationAndBounds(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	if data, err := PackBC1NormalDepth(ctx, []byte{128, 128, 255, 127}); data != nil || !errors.Is(err, context.Canceled) {
		t.Fatal("canceled packing returned data", data, err)
	}

	if data, err := PackBC1NormalDepth(t.Context(), []byte{1, 2, 3}); data != nil || !errors.Is(err, errBounds) {
		t.Fatal("partial texel was admitted", data, err)
	}
}
