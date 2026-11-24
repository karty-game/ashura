//go:build linux

package native

import (
	"github.com/karty-game/ashura/internal/particledata"
	wgpu "github.com/karty-game/ashura/internal/webgpu"
)

type particleWarmCopy struct {
	source, destination *wgpu.Buffer
	step                *particledata.Step
	size                uint64
}

func (feature *particleFeature) prepareWarm(source *particledata.Layer, state *wgpu.Buffer, step *particledata.Step) error {
	size := uint64(len(source.Warm))

	buffer := feature.warmBuffers[source]
	if buffer == nil {
		var err error

		buffer, err = feature.device.CreateBuffer(&wgpu.BufferDescriptor{
			Label: "Prepared warm particle state", Size: size, Usage: wgpu.BufferUsageCopySrc | wgpu.BufferUsageCopyDst,
		})
		if err != nil {
			return err
		}

		feature.warmBuffers[source] = buffer
		if err := feature.queue.WriteBuffer(buffer, 0, source.Warm); err != nil {
			return err
		}
	}

	feature.warmCopies = append(feature.warmCopies, particleWarmCopy{
		source: buffer, destination: state, step: step, size: size,
	})

	return nil
}

func (feature *particleFeature) copyWarmOnReset(encoder *wgpu.CommandEncoder) {
	for _, warm := range feature.warmCopies {
		if warm.step.Reset {
			encoder.CopyBufferToBuffer(warm.source, 0, warm.destination, 0, warm.size)
		}
	}
}
