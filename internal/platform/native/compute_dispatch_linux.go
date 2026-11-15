//go:build linux

package native

import (
	"fmt"

	"github.com/karty-game/karty-webgpu/internal/computedata"
	"github.com/karty-game/karty-webgpu/internal/diagnostics"
)

// Surface loss and zero-sized windows skip presentation, but one-shot compute
// commands have already been accepted by the App and must still be submitted.
// Empty schedules create no encoder and never wait for GPU completion.
func (driver *computeBackend) submitComputeOnly(schedule []computedata.Dispatch) error {
	if len(schedule) == 0 {
		return nil
	}

	encoder, err := driver.device.CreateCommandEncoder(nil)
	if err != nil {
		return fmt.Errorf("create compute-only encoder: %w", err)
	}
	defer encoder.Release()

	if err := driver.recordCompute(encoder, schedule); err != nil {
		return err
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
