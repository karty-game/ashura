//go:build !js

package lightmapper

import (
	"bytes"
	"encoding/binary"
	"errors"
	"math"
	"testing"
)

func TestGPUVerificationAdmitsOnlyCompleteCanonicalSeedTails(t *testing.T) {
	t.Parallel()
	scene := gpuFixtureScene(t, Scene{Environment: Environment{Sky: Vec3{.2, .2, .2}}})
	receivers := []Receiver{gpuFixtureReceiver(1, 0), gpuFixtureReceiver(2, 1)}
	normals := [][]Vec3{{{0, 0, 1}}, {{0, 0, 1}}}

	tile, err := PackGPUVerification(scene, receivers, Options{Samples: 4, Outputs: OutputRGB | OutputRNM}, normals)
	if err != nil {
		t.Fatal(err)
	}

	packed, err := PackGPUScene(scene)
	if err != nil {
		t.Fatal(err)
	}

	if err = gpuValidatePackets(packed, tile); err != nil {
		t.Fatal("legacy uniform seed rejected", err)
	}

	seedOffset := gpuOffsetWord(len(tile.Data) / gpuWordBytes)
	binary.LittleEndian.PutUint32(tile.Data[24:], seedOffset)
	binary.LittleEndian.PutUint32(tile.Data[28:], tile.Count)
	tile.Data = binary.LittleEndian.AppendUint32(tile.Data, 29)

	tile.Data = binary.LittleEndian.AppendUint32(tile.Data, 71)
	if err = gpuValidatePackets(packed, tile); err != nil {
		t.Fatal("complete explicit seed tail rejected", err)
	}

	for _, test := range []struct {
		name string
		edit func(*GPUTile)
	}{
		{"offset-overlap", func(v *GPUTile) { binary.LittleEndian.PutUint32(v.Data[24:], seedOffset-1) }},
		{"offset-overflow", func(v *GPUTile) { binary.LittleEndian.PutUint32(v.Data[24:], math.MaxUint32) }},
		{"seed-count", func(v *GPUTile) { binary.LittleEndian.PutUint32(v.Data[28:], v.Count+1) }},
		{"legacy-count", func(v *GPUTile) { binary.LittleEndian.PutUint32(v.Data[24:], 0) }},
		{"legacy-tail", func(v *GPUTile) {
			binary.LittleEndian.PutUint32(v.Data[24:], 0)
			binary.LittleEndian.PutUint32(v.Data[28:], 0)
		}},
		{"missing-seed", func(v *GPUTile) { v.Data = v.Data[:len(v.Data)-gpuWordBytes] }},
		{"extra-seed", func(v *GPUTile) { v.Data = binary.LittleEndian.AppendUint32(v.Data, 1) }},
		{"probe-offset", func(v *GPUTile) { binary.LittleEndian.PutUint32(v.Data[16:], 0) }},
		{"patch-range", func(v *GPUTile) { binary.LittleEndian.PutUint32(v.Data[12:], math.MaxUint32) }},
		{"probe-product", func(v *GPUTile) {
			v.ProbeCount = math.MaxUint32
			binary.LittleEndian.PutUint32(v.Data[20:], v.ProbeCount)
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			invalid := tile
			invalid.Data = bytes.Clone(tile.Data)
			test.edit(&invalid)

			if err := gpuValidatePackets(packed, invalid); !errors.Is(err, ErrInvalid) {
				t.Fatal("malformed seed/probe packet admitted", err)
			}
		})
	}
}
