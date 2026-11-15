//go:build !js

package levelbuild

import (
	"runtime"
	"sync"

	"github.com/karty-game/karty-webgpu/pkg/assetbuild"
	bake "github.com/karty-game/karty-webgpu/pkg/lightmap/bake"
)

// Page bytes are immutable and their numeric admission is independent. Only
// tracked Context access is serialized: it appends the action's dependencies.
// Results and errors retain page order regardless of decode completion order.
func (plugin *productionPlugin) readProductionPages(
	ctx *assetbuild.Context,
	scene *bake.PreparedScene,
	pages []pageRecord,
	samples []assetbuild.Ref,
) ([]bake.ReceiverSet, []bake.Result, error) {
	sets := make([]bake.ReceiverSet, len(pages))
	results := make([]bake.Result, len(pages))
	errors := make([]error, len(pages))
	reader := productionPageReader{ctx: ctx, plugin: plugin}
	jobs := make(chan int)

	var workers sync.WaitGroup

	for range min(len(pages), runtime.GOMAXPROCS(0)) {
		workers.Go(func() {
			for index := range jobs {
				sets[index], results[index], errors[index] = reader.read(scene, pages[index], samples[index])
			}
		})
	}

	for index := range pages {
		jobs <- index
	}

	close(jobs)
	workers.Wait()

	for _, err := range errors {
		if err != nil {
			return nil, nil, err
		}
	}

	return sets, results, nil
}

type productionPageReader struct {
	plugin *productionPlugin
	ctx    *assetbuild.Context
	mu     sync.Mutex
}

func (reader *productionPageReader) artifact(artifact assetbuild.Artifact) ([]byte, error) {
	reader.mu.Lock()
	defer reader.mu.Unlock()

	return reader.ctx.ReadArtifact(artifact)
}

func (reader *productionPageReader) sample(ref assetbuild.Ref) (productionSample, error) {
	reader.mu.Lock()
	defer reader.mu.Unlock()

	var sample productionSample

	err := readRecord(reader.ctx, ref, &sample)

	return sample, err
}

func (reader *productionPageReader) read(
	scene *bake.PreparedScene, page pageRecord, sampleRef assetbuild.Ref,
) (bake.ReceiverSet, bake.Result, error) {
	set, err := reader.plugin.readProductionReceiverPage(reader.ctx, scene, page, reader.artifact)
	if err != nil {
		return bake.ReceiverSet{}, bake.Result{}, err
	}

	sample, err := reader.sample(sampleRef)
	if err != nil {
		return bake.ReceiverSet{}, bake.Result{}, err
	}

	data, err := reader.artifact(sample.Linear)
	if err != nil {
		return bake.ReceiverSet{}, bake.Result{}, err
	}

	result, err := bake.DecodeResult(data)
	if err != nil {
		return bake.ReceiverSet{}, bake.Result{}, err
	}

	return set, result, nil
}
