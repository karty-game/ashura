//go:build !js

package lightmapper

import (
	"encoding/binary"
	"math"
)

const (
	gpuSceneHeaderWords = 64
	gpuPathWords        = 128
	gpuResultWords      = 56
	gpuNodeWords        = 20
	gpuLightNodeWords   = 16
	gpuTriangleWords    = 32
	gpuMaterialWords    = 16
	gpuEmitterWords     = 32
	gpuPatchWords       = 40
	gpuMaximumBytes     = 128 << 20
)

type gpuWords []uint32

func (w *gpuWords) reserve(n int) int {
	at := len(*w)
	*w = append(*w, make([]uint32, n)...)

	return at
}
func (w *gpuWords) vec(at int, v Vec3) {
	for i := range 3 {
		(*w)[at+i] = math.Float32bits(v[i])
	}
}
func (w *gpuWords) bytes() []byte {
	data := make([]byte, len(*w)*4)
	for i, v := range *w {
		binary.LittleEndian.PutUint32(data[i*4:], v)
	}

	return data
}

// PackGPUScene writes the versioned checked storage ABI. Its hash identifies
// immutable prepared content; the worker keeps that GPU scene between tiles.
func PackGPUScene(scene *PreparedScene) (GPUScene, error) {
	if scene == nil {
		return GPUScene{}, ErrInvalid
	}

	count := gpuSceneWordCount(scene)
	if count*gpuWordBytes > gpuMaximumBytes {
		return GPUScene{}, ErrBounds
	}

	build, err := gpuBuildWide(scene, int(gpuMaximumBytes/gpuWordBytes-count))
	if err != nil {
		return GPUScene{}, err
	}

	words := make(gpuWords, gpuSceneHeaderWords, int(count)+len(build.packed))
	words = gpuPackSceneHeader(words, scene)
	words = gpuPackWideNodes(words, build)
	words = gpuPackSceneTriangles(words, scene)
	words = gpuPackSceneMaterials(words, scene)
	words = gpuPackSceneTextures(words, scene)
	words = gpuPackSceneEmitters(words, scene)
	words = gpuPackSceneLights(words, scene)

	words = gpuPackSceneEnvironment(words, scene)
	words = gpuPackRayTriangles(words, scene, build.order)

	if len(words)*gpuWordBytes > gpuMaximumBytes {
		return GPUScene{}, ErrBounds
	}

	packet := GPUScene{Key: scene.hash, Data: words.bytes()}
	if !gpuValidateWideScene(packet.Data) {
		return GPUScene{}, ErrInvalid
	}

	return packet, nil
}

// PackGPUTile retains covered UV area and separate front/back receiver identity.
func PackGPUTile(scene *PreparedScene, receivers []Receiver, options Options) (GPUTile, error) {
	if scene == nil || len(receivers) == 0 || len(receivers) > 1<<20 {
		return GPUTile{}, ErrInvalid
	}

	capacity, err := gpuTileWordCount(receivers)
	if err != nil {
		return GPUTile{}, err
	}

	//nolint:gosec // G115: gpuTileWordCount checked capacity <=128MiB/4, which fits the portable int range.
	words := make(gpuWords, gpuTileHeaderWords+len(receivers)*gpuReceiverWords, int(capacity))
	words[0] = gpuOffsetWord(len(receivers))
	words[1] = gpuTileHeaderWords
	words[2] = gpuOffsetWord(len(words))

	ids := make(map[uint64]bool, len(receivers))
	for index, receiver := range receivers {
		if ids[receiver.ID] {
			return GPUTile{}, ErrInvalid
		}

		ids[receiver.ID] = true

		words, err = gpuPackReceiver(words, scene, receiver, gpuTileHeaderWords+index*gpuReceiverWords)
		if err != nil {
			return GPUTile{}, err
		}
	}

	return GPUTile{Data: words.bytes(), Count: gpuOffsetWord(len(receivers)), Options: options}, nil
}
