//go:build !js

package lightmapper

import (
	"context"
)

// Bake executes fixed-count production sampling on the admitted hardware. Scene
// packets are cached for the worker lifetime; memory scheduling never changes
// receiver identity, random sequence, sample count or quality policy.
func (w *GPUWorker) Bake(ctx context.Context, scene *PreparedScene, receivers []Receiver, options Options) (Result, error) {
	results, err := w.BakeBatches(ctx, scene, []BakeBatch{{Receivers: receivers, Options: options}})
	if err != nil {
		return Result{}, err
	}

	return results[0], nil
}

func (w *GPUWorker) scenePacket(scene *PreparedScene) (GPUScene, error) {
	w.mu.Lock()
	if w.packedScene.Key == scene.Hash() {
		packet := w.packedScene
		w.mu.Unlock()

		return packet, nil
	}
	w.mu.Unlock()

	packet, err := PackGPUScene(scene)
	if err != nil {
		return GPUScene{}, err
	}

	w.mu.Lock()
	w.packedScene = packet
	w.mu.Unlock()

	return packet, nil
}

func gpuEstimateCountsMatch(estimates []Estimate, count uint32) bool {
	for _, estimate := range estimates {
		if estimate.Count != count {
			return false
		}
	}

	return true
}
