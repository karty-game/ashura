//go:build !js

package basiscodec

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"testing"
	"time"

	"github.com/tetratelabs/wazero/api"
)

//nolint:paralleltest // Serial codec qualification bounds simultaneous VM memory.
func TestSessionHDRGolden(t *testing.T) {
	session := New()
	defer func() { _ = session.Close() }()

	palette := [4][3]float32{{.25, .5, 1}, {4, 2, 1}, {16, 8, 2}, {64, 32, 8}}

	var pixels []byte

	for y := range 16 {
		for x := range 16 {
			color := palette[(x/4+(y/4)*2)%4]
			for _, value := range [4]float32{color[0], color[1], color[2], 1} {
				pixels = binary.LittleEndian.AppendUint32(pixels, math.Float32bits(value))
			}
		}
	}

	encoded, err := session.Encode(t.Context(), Image{Width: 16, Height: 16, Levels: [][]byte{pixels}, HDR: true}, 0, false)
	if err != nil {
		t.Fatal(err)
	}

	result, err := session.Transcode(t.Context(), encoded, 22)
	if err != nil || result.Width != 16 || result.Height != 16 || result.SRGB || len(result.Levels) != 1 {
		t.Fatal(result, err)
	}

	if actual := fmt.Sprintf(
		"%x",
		sha256.Sum256(result.Levels[0]),
	); actual != "785a8cd5eb5b4a4d25b3d7cdb82623bf6b0ac6392a8309956a5721bbd0ff7577" {
		t.Fatal("qualified BC6H blocks changed", actual)
	}
}

//nolint:paralleltest // Serial codec qualification bounds simultaneous VM memory.
func TestSessionAuthoredMipsAndCancellation(t *testing.T) {
	session := New()
	defer func() { _ = session.Close() }()

	var (
		encoded []byte
		result  Result
		err     error
		encoder api.Module
	)

	levels := make([][]byte, 0, 5)
	for i, extent := range []int{16, 8, 4, 2, 1} {
		data := make([]byte, extent*extent*4)
		for offset := 0; offset < len(data); offset += 4 {
			copy(data[offset:], []byte{[]byte{20, 55, 90, 125, 160}[i], 80, 160, []byte{25, 70, 115, 160, 205}[i]})
		}

		levels = append(levels, data)
	}

	for _, srgb := range []bool{false, true} {
		encoded, err = session.Encode(t.Context(), Image{Width: 16, Height: 16, Levels: levels, SRGB: srgb}, 4, false)
		if err != nil {
			t.Fatal(err)
		}

		if encoder != nil && session.encoder != encoder {
			t.Fatal("encoder was not reused")
		}

		encoder = session.encoder
		for _, target := range []uint64{0, 1, 2, 6, 10, 13} {
			result, err = session.Transcode(t.Context(), encoded, target)
			if err != nil || result.Width != 16 || result.Height != 16 || result.SRGB != srgb || len(result.Levels) != 5 {
				t.Fatal(target, result, err)
			}

			if target == 13 {
				checkDecodedMips(t, result.Levels, levels)
			}
		}
	}

	bounded, stop := context.WithTimeout(t.Context(), 25*time.Millisecond)
	started := time.Now()
	_, failure := session.Encode(bounded, Image{Width: 512, Height: 512, Levels: [][]byte{make([]byte, 512*512*4)}}, 4, false)

	stop()

	if !errors.Is(failure, context.DeadlineExceeded) || time.Since(started) > time.Second {
		t.Fatal("active codec did not cancel promptly", failure, time.Since(started))
	}

	if _, failure = session.Encode(t.Context(), Image{Width: 16, Height: 16, Levels: levels}, 0, false); failure != nil {
		t.Fatal("canceled VM prevented recovery", failure)
	}

	if _, err = session.Transcode(t.Context(), make([]byte, 80), 6); err == nil {
		t.Fatal("accepted malformed KTX2")
	}

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	if _, err = session.Encode(ctx, Image{Width: 16, Height: 16, Levels: levels}, 0, false); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}

	if err = session.Close(); err != nil {
		t.Fatal(err)
	}

	if _, err = session.Encode(t.Context(), Image{Width: 16, Height: 16, Levels: levels}, 0, false); !errors.Is(err, ErrInvalid) {
		t.Fatal("closed session accepted work", err)
	}
}

func checkDecodedMips(t *testing.T, actual, expected [][]byte) {
	t.Helper()

	for mipIndex, mip := range actual {
		if len(mip) != len(expected[mipIndex]) {
			t.Fatal("mip dimensions changed")
		}

		for index, value := range mip {
			if math.Abs(float64(value)-float64(expected[mipIndex][index])) > 3 {
				t.Fatal("authored numeric mip/alpha changed", mipIndex, index)
			}
		}
	}
}
