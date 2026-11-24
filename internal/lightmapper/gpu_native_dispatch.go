//go:build !js

//nolint:mnd // Buffer layouts, flag words and dispatch widths mirror the production WGSL ABI.
package lightmapper

import (
	"context"
	"encoding/binary"

	wgpu "github.com/karty-game/ashura/internal/webgpu"
)

func (executor *nativeGPUExecutor) encode(ctx context.Context, operation func(*wgpu.CommandEncoder) error) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	encoder, err := executor.gpu.CreateCommandEncoder(nil)
	if err != nil {
		return err
	}
	defer encoder.Release()

	if err = operation(encoder); err != nil {
		return err
	}

	command, err := encoder.Finish()
	if err != nil {
		return err
	}
	defer command.Release()

	_, err = executor.queue.Submit(command)

	return err
}
func (executor *nativeGPUExecutor) stage(encoder *wgpu.CommandEncoder, tile GPUTile, batch *nativeBatch, stage int) error {
	pass, err := encoder.BeginComputePass(&wgpu.ComputePassDescriptor{Label: nativeStageName(stage)})
	if err != nil {
		return err
	}
	defer pass.Release()

	pass.SetPipeline(executor.stages[stage])
	pass.SetBindGroup(0, batch.groups[stageLayout(stage)], nil)

	switch stage {
	case stageReset, stageDispatch:
		pass.DispatchWorkgroups(1, 1, 1)
	case stageGenerate:
		pass.DispatchWorkgroups((tile.Count*batch.sizes.pool+63)/64, 1, 1)
	case stageReduce, stageResolve, stageReconstruct, stageSerialize:
		pass.DispatchWorkgroups((tile.Count+63)/64, 1, 1)
	default:
		pass.DispatchWorkgroupsIndirect(batch.indirect, 0)
	}

	pass.End()

	return nil
}
func (executor *nativeGPUExecutor) direct(encoder *wgpu.CommandEncoder, tile GPUTile, batch *nativeBatch) error {
	if tile.Options.ExactLights {
		if err := executor.stage(encoder, tile, batch, stageExact); err != nil {
			return err
		}

		return executor.stage(encoder, tile, batch, stageContinue)
	}

	return executor.stage(encoder, tile, batch, stageAdvance)
}
func (executor *nativeGPUExecutor) samplePasses(encoder *wgpu.CommandEncoder, tile GPUTile, batch *nativeBatch) error {
	for _, stage := range []int{stageReset, stageGenerate} {
		if err := executor.stage(encoder, tile, batch, stage); err != nil {
			return err
		}
	}

	if err := executor.direct(encoder, tile, batch); err != nil {
		return err
	}

	for depth := uint32(0); ; depth++ {
		for _, stage := range []int{stageCompact, stageDispatch} {
			if err := executor.stage(encoder, tile, batch, stage); err != nil {
				return err
			}
		}

		if depth > tile.Options.Bounces {
			break
		}

		if err := executor.stage(encoder, tile, batch, stageIntersect); err != nil {
			return err
		}

		if err := executor.direct(encoder, tile, batch); err != nil {
			return err
		}
	}

	return executor.stage(encoder, tile, batch, stageReduce)
}
func nativeUniforms(tile GPUTile, pool uint32) [64]byte {
	var words [16]uint32

	words[0], words[2], words[3], words[4], words[5], words[7], words[8], words[12] = tile.Count, tile.Options.Samples,
		tile.Options.Bounces, tile.Options.Seed, tile.Options.RouletteDepth, pool, uint32(tile.Options.Outputs), tile.ProbeCount
	if tile.Options.ExactLights {
		words[6] = 1
	}

	if tile.Mode == gpuModeVerify {
		words[9] = 1
	}

	var data [64]byte
	for index, word := range words {
		binary.LittleEndian.PutUint32(data[index*4:], word)
	}

	return data
}
func (executor *nativeGPUExecutor) execute(ctx context.Context, tile GPUTile, batch *nativeBatch) error {
	uniforms := nativeUniforms(tile, batch.sizes.pool)
	if tile.Mode == gpuModeReconstruct {
		if err := executor.queue.WriteBuffer(batch.uniform, 0, uniforms[:]); err != nil {
			return err
		}

		if err := executor.encode(ctx, func(encoder *wgpu.CommandEncoder) error {
			return executor.stage(encoder, tile, batch, stageReconstruct)
		}); err != nil {
			return err
		}
	} else if err := executor.executeSamples(ctx, tile, batch, uniforms); err != nil {
		return err
	}

	return executor.encode(ctx, func(encoder *wgpu.CommandEncoder) error {
		source, offset := batch.output, uint64(0)
		if tile.Mode == "" {
			source, offset = batch.paths, gpuPathHeaderBytes
		}

		encoder.CopyBufferToBuffer(source, offset, batch.readback, 0, batch.sizes.output)

		if executor.traceRays && tile.Mode != gpuModeReconstruct {
			count := uint64(tile.Count)
			for plane := range uint64(4) {
				encoder.CopyBufferToBuffer(batch.paths, gpuPathHeaderBytes+(96+plane)*count*uint64(batch.sizes.pool)*4,
					batch.readback, batch.sizes.output+plane*count*4, count*4)
			}

			encoder.CopyBufferToBuffer(batch.paths, 12, batch.readback, batch.sizes.output+count*16, 4)
		}

		return nil
	})
}
func (executor *nativeGPUExecutor) executeSamples(ctx context.Context, tile GPUTile, batch *nativeBatch, uniforms [64]byte) error {
	for sample := uint32(0); sample < tile.Options.Samples; sample += batch.sizes.pool {
		if err := ctx.Err(); err != nil {
			return err
		}

		binary.LittleEndian.PutUint32(uniforms[4:], sample)

		if err := executor.queue.WriteBuffer(batch.uniform, 0, uniforms[:]); err != nil {
			return err
		}

		if err := executor.encode(
			ctx,
			func(encoder *wgpu.CommandEncoder) error { return executor.samplePasses(encoder, tile, batch) },
		); err != nil {
			return err
		}

		if (sample/batch.sizes.pool+1)%8 == 0 {
			// Bound queued command storage using the existing readback allocation as
			// a fence. Nonblocking mapping lets cancellation interrupt this wait.
			if err := executor.encode(ctx, func(encoder *wgpu.CommandEncoder) error {
				encoder.CopyBufferToBuffer(batch.output, 0, batch.readback, 0, gpuWordBytes)

				return nil
			}); err != nil {
				return err
			}

			if err := executor.mapReadback(ctx, batch.readback, gpuWordBytes); err != nil {
				return err
			}

			if err := batch.readback.Unmap(); err != nil {
				return err
			}
		}
	}

	return executor.encode(ctx, func(encoder *wgpu.CommandEncoder) error {
		if err := executor.stage(encoder, tile, batch, stageResolve); err != nil {
			return err
		}

		if tile.Mode == "" {
			return executor.stage(encoder, tile, batch, stageSerialize)
		}

		return nil
	})
}
