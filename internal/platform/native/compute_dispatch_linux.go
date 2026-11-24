//go:build linux

package native

import (
	"fmt"

	"github.com/karty-game/ashura/internal/diagnostics"
	"github.com/karty-game/ashura/internal/particledata"
	"github.com/karty-game/ashura/internal/render"
)

// Surface loss and zero-sized windows skip presentation, but one-shot compute
// commands have already been accepted by the App and must still be submitted.
// Empty schedules create no encoder and never wait for GPU completion.
func (driver *Driver) submitComputeOnly(frame render.Frame) error {
	particles := driver.particleBridge != nil && particledata.HasWork(frame.Particles)
	if len(frame.Compute) == 0 && !particles {
		return nil
	}

	encoder, err := driver.device.CreateCommandEncoder(nil)
	if err != nil {
		return fmt.Errorf("create compute-only encoder: %w", err)
	}
	defer encoder.Release()

	if err := driver.recordCompute(encoder, frame.Compute); err != nil {
		return err
	}

	if particles {
		if err := driver.particleBridge.Simulate(encoder, frame.Particles); err != nil {
			return err
		}
	}

	driver.metrics.Step(diagnostics.ComputePhase)

	commands, err := encoder.Finish()
	if err != nil {
		return fmt.Errorf("finish compute-only encoder: %w", err)
	}
	defer commands.Release()

	if _, err := driver.queue.Submit(commands); err != nil {
		return fmt.Errorf("submit compute without presentation: %w", err)
	}

	driver.metrics.Step(diagnostics.SubmitPhase)

	return nil
}
