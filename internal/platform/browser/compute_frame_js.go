//go:build js && wasm

package browser

import (
	"fmt"
	"syscall/js"

	"github.com/karty-game/karty-webgpu/internal/bufferdata"
	"github.com/karty-game/karty-webgpu/internal/computedata"
)

func (driver *computeBackend) uploadComputeBuffers(uploads []bufferdata.Upload) error {
	if len(uploads) == 0 {
		return nil
	}

	queue := driver.queue
	for _, upload := range uploads {
		cached, ok := driver.computeBuffers[upload.Buffer]
		if !ok {
			return errComputeResource
		}

		js.CopyBytesToJS(cached.array, upload.Data)
		queue.Call("writeBuffer", cached.buffer, float64(upload.Offset), cached.array, 0, len(upload.Data))
	}

	return nil
}

func (driver *computeBackend) recordCompute(encoder js.Value, commands []computedata.Dispatch) error {
	if len(commands) == 0 {
		return nil
	}

	pass := encoder.Call("beginComputePass")

	for _, command := range commands {
		pipeline, hasPipeline := driver.computePipelines[command.Pipeline]

		group, hasGroup := driver.computeBindings[command.Group]
		if !hasPipeline || !hasGroup {
			pass.Call("end")

			return errComputeResource
		}

		if command.X > driver.computeWorkgroups || command.Y > driver.computeWorkgroups || command.Z > driver.computeWorkgroups {
			pass.Call("end")

			return fmt.Errorf("%w: dispatch workgroup count", errComputeLimit)
		}

		pass.Call("setPipeline", pipeline.pipeline)
		pass.Call("setBindGroup", 0, group)
		pass.Call("dispatchWorkgroups", command.X, command.Y, command.Z)
		driver.metrics.Dispatch()
	}

	pass.Call("end")

	return nil
}

// UnloadBuffer drops its cached upload storage after bindings are detached.
func (driver *computeBackend) UnloadBuffer(source *bufferdata.Buffer) {
	if cached, ok := driver.computeBuffers[source]; ok {
		cached.buffer.Call("destroy")
		delete(driver.computeBuffers, source)
	}
}

// UnloadComputePipeline drops reusable pipeline state; queued work retains its refs.
func (driver *computeBackend) UnloadComputePipeline(source *computedata.Pipeline) {
	delete(driver.computePipelines, source)
}

// UnloadComputeBindings drops a reusable group after it is detached from scheduling.
func (driver *computeBackend) UnloadComputeBindings(source *computedata.BindGroup) {
	delete(driver.computeBindings, source)
}

func (driver *computeBackend) Close() {
	clear(driver.computeBindings)
	clear(driver.computePipelines)
	clear(driver.computeModules)

	for source := range driver.computeBuffers {
		driver.UnloadBuffer(source)
	}
}
