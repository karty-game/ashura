//go:build !js

package lightmapper

import "errors"

// gpuBuildWide replaces GPU-only binary tables. CPU reference acceleration and
// the original primitive priority remain owned by PreparedScene.
func gpuBuildWide(scene *PreparedScene, wordBudget int) (*gpuWideBuilder, error) {
	build := &gpuWideBuilder{scene: scene, oldOrder: scene.order}
	if len(scene.order) <= gpuWideLeafPrimitives {
		build.order = scene.order

		return build, nil
	}

	build.packed = make([]uint32, gpuNodeWords)
	sah := gpuWideSAH{b: build}
	sah.prepare()

	root := sah.build(append([]uint32(nil), scene.order...), 0)
	if err := build.buildBounded(root, gpuWideMaximumDepth, wordBudget); err != nil {
		if !errors.Is(err, ErrBounds) {
			return nil, err
		}
		// A representable direct-AS remains available when quantized bounds
		// cannot be represented or cannot fit the actual remaining byte budget.
		build.packed = nil
		build.order = scene.order
		build.depth = 0
		build.linear = true
		build.err = nil
	}

	return build, nil
}

func gpuPackWideNodes(words gpuWords, build *gpuWideBuilder) gpuWords {
	words[2] = gpuOffsetWord(len(words))
	words[3] = gpuOffsetWord(len(build.packed) / gpuNodeWords)

	words[gpuWideDepthWord] = gpuOffsetWord(build.depth)
	if build.linear {
		words[gpuWideModeWord] = 1
	}

	return append(words, build.packed...)
}
