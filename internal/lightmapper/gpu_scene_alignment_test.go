//go:build !js

package lightmapper

import (
	"encoding/binary"
	"errors"
	"testing"
)

func TestGPUSceneVectorAlignmentAndAdmission(t *testing.T) {
	t.Parallel()

	for count := range 7 {
		source := testCoreScene()
		source.Materials = []Material{{DoubleSided: true}}

		for index := range count {
			z := float32(index)
			source.Triangles = append(
				source.Triangles,
				Triangle{ID: uint64(index + 1), Positions: [3]Vec3{{0, 0, z}, {1, 0, z}, {0, 1, z}}},
			)
		}

		scene, err := PrepareScene(source)
		if err != nil {
			t.Fatal(err)
		}

		packed, err := PackGPUScene(scene)
		if err != nil {
			t.Fatal(err)
		}

		tile, err := PackGPUTile(scene, []Receiver{testPointReceiver()}, Options{Samples: 1, Outputs: OutputRGB})
		if err != nil {
			t.Fatal(err)
		}

		word := func(at int) uint32 { return binary.LittleEndian.Uint32(packed.Data[at*4:]) }
		if len(packed.Data)%16 != 0 || word(2)%4 != 0 || word(33)%4 != 0 ||
			uint64(word(33))+uint64(count)*12 != uint64(len(packed.Data)/4) {
			t.Fatal("unaligned vector scene", count, len(packed.Data), word(2), word(33))
		}

		if err = gpuValidatePackets(packed, tile); err != nil {
			t.Fatal("valid packet", count, err)
		}

		changes := []func(*GPUScene){
			func(v *GPUScene) { v.Data = append(v.Data, 0, 0, 0, 0) },
			func(v *GPUScene) { binary.LittleEndian.PutUint32(v.Data[2*4:], word(2)+1) },
			func(v *GPUScene) { binary.LittleEndian.PutUint32(v.Data[33*4:], word(33)+1) },
			func(v *GPUScene) { binary.LittleEndian.PutUint32(v.Data[3*4:], ^uint32(0)) },
			func(v *GPUScene) { binary.LittleEndian.PutUint32(v.Data[5*4:], ^uint32(0)) },
			func(v *GPUScene) { binary.LittleEndian.PutUint32(v.Data[33*4:], ^uint32(0)-3) },
		}
		for index, change := range changes {
			bad := packed
			bad.Data = append([]byte(nil), packed.Data...)
			change(&bad)

			if err = gpuValidatePackets(bad, tile); !errors.Is(err, ErrInvalid) {
				t.Fatal("malformed vector packet admitted", count, index, err)
			}
		}
	}
}
