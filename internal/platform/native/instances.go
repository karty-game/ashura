//go:build linux

package native

import (
	"fmt"

	"github.com/gogpu/gputypes"
	"github.com/karty-game/karty-webgpu/internal/render"
	wgpu "github.com/karty-game/karty-webgpu/internal/webgpu"
)

type gpuInstances struct {
	buffer   *wgpu.Buffer
	capacity int
	state    render.InstanceState
	seen     bool
}

func (driver *Driver) instanceBuffer(draw render.DrawCall) (*gpuInstances, error) {
	if driver.instances == nil {
		driver.instances = make(map[uint32]*gpuInstances)
	}

	entry := driver.instances[draw.ID]
	if entry == nil {
		entry = &gpuInstances{}
		driver.instances[draw.ID] = entry
	}

	entry.state.Update(draw)

	if len(entry.state.Data) > entry.capacity {
		capacity := bufferCapacity(len(entry.state.Data))

		buffer, err := driver.device.CreateBuffer(
			&wgpu.BufferDescriptor{
				Label: "draw instances",
				Usage: gputypes.BufferUsageVertex | gputypes.BufferUsageCopyDst,
				Size:  uint64(capacity), //nolint:gosec // Capacity grows from 256 and is bounded by admitted 128MiB payloads.
			},
		)
		if err != nil {
			return nil, fmt.Errorf("create instance buffer: %w", err)
		}

		if entry.buffer != nil {
			entry.buffer.Release()
		}

		entry.buffer, entry.capacity = buffer, capacity
		entry.state.All()
	}

	for _, span := range entry.state.Dirty {
		offset := uint64(span.Offset) //nolint:gosec // Dirty byte offsets come from admitted nonnegative matrix indices.
		if err := driver.queue.
			WriteBuffer(
				entry.buffer,
				offset,
				entry.state.Data[span.Offset:span.End],
			); err != nil {
			return nil, fmt.Errorf("write instance buffer: %w", err)
		}
	}

	entry.seen = true

	return entry, nil
}

func bufferCapacity(required int) int {
	capacity := 256
	for capacity < required {
		capacity *= 2
	}

	return capacity
}

func (driver *Driver) pruneInstances() {
	for identifier, entry := range driver.instances {
		if !entry.seen {
			if entry.buffer != nil {
				entry.buffer.Release()
			}

			delete(driver.instances, identifier)
		}

		entry.seen = false
	}
}

func (driver *Driver) releaseInstances() {
	for identifier, entry := range driver.instances {
		if entry.buffer != nil {
			entry.buffer.Release()
		}

		delete(driver.instances, identifier)
	}
}
