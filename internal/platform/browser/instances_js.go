//go:build js && wasm

package browser

import (
	"syscall/js"

	"github.com/karty-game/ashura/internal/render"
)

type gpuInstances struct {
	buffer, array js.Value
	capacity      int
	state         render.InstanceState
	seen          bool
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
		const usage = 0x28

		capacity := nextCapacity(len(entry.state.Data))

		buffer := driver.device.Call(
			"createBuffer",
			jsObject(map[string]any{labelProperty: "draw instances", sizeProperty: capacity, usageProperty: usage}),
		)
		if buffer.IsUndefined() || buffer.IsNull() {
			return nil, errDrawBuffer
		}

		if !entry.buffer.IsUndefined() && !entry.buffer.IsNull() {
			entry.buffer.Call("destroy")
		}

		entry.buffer, entry.capacity = buffer, capacity
		entry.array = js.Global().Get("Uint8Array").New(capacity)
		entry.state.All()
	}

	for _, span := range entry.state.Dirty {
		js.CopyBytesToJS(entry.array, entry.state.Data[span.Offset:span.End])
		driver.queue.Call("writeBuffer", entry.buffer, span.Offset, entry.array, 0, span.End-span.Offset)
	}

	entry.seen = true

	return entry, nil
}

func (driver *Driver) pruneInstances() {
	for identifier, entry := range driver.instances {
		if !entry.seen {
			if !entry.buffer.IsUndefined() && !entry.buffer.IsNull() {
				entry.buffer.Call("destroy")
			}

			delete(driver.instances, identifier)
		}

		entry.seen = false
	}
}

func (driver *Driver) releaseInstances() {
	for identifier, entry := range driver.instances {
		if !entry.buffer.IsUndefined() && !entry.buffer.IsNull() {
			entry.buffer.Call("destroy")
		}

		delete(driver.instances, identifier)
	}
}
