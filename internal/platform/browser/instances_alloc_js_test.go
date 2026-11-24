//go:build js && wasm

package browser

import (
	"bytes"
	"syscall/js"
	"testing"

	"github.com/karty-game/ashura/internal/render"
)

//nolint:paralleltest // AllocsPerRun must remain isolated on Go 1.27.
func TestSparseInstanceUploadsReuseStagingWithoutGoAllocations(t *testing.T) {
	factory := js.Global().Get("Function").New(`return {
  createBuffer(desc) { return { data: new Uint8Array(desc.size) }; },
  calls: 0, bytes: 0,
  writeBuffer(buffer, offset, data, start = 0, size = data.byteLength - start) {
   buffer.data.set(data.subarray(start, start + size), offset);
   this.calls++; this.bytes += size;
  }
 }`)
	device := factory.Invoke()
	driver := &Driver{device: device, queue: device}
	draw := render.DrawCall{ID: 1, Instances: make([]render.InstanceTransform, 9)}

	entry, err := driver.instanceBuffer(draw)
	if err != nil {
		t.Fatal(err)
	}

	device.Set("calls", 0)
	device.Set("bytes", 0)

	draw.Instances[1].Model[12] = 2
	draw.Instances[7].Model[13] = 4

	if _, err := driver.instanceBuffer(draw); err != nil {
		t.Fatal(err)
	}

	if device.Get("calls").Int() != 2 || device.Get("bytes").Int() != 2*render.InstanceStride {
		t.Fatal("sparse edits did not upload only their two affected ranges")
	}

	actual := make([]byte, len(entry.state.Data))
	js.CopyBytesToGo(actual, entry.buffer.Get("data"))

	if !bytes.Equal(actual, entry.state.Data) {
		t.Fatal("sparse uploads changed the wrong destination offsets or reused stale staging bytes")
	}

	device.Set("calls", 0)

	if _, err := driver.instanceBuffer(draw); err != nil {
		t.Fatal(err)
	}

	if device.Get("calls").Int() != 0 {
		t.Fatal("unchanged instances uploaded again")
	}

	var updateErr error

	allocations := testing.AllocsPerRun(100, func() {
		draw.Instances[1].Model[12]++
		draw.Instances[7].Model[13]++
		_, updateErr = driver.instanceBuffer(draw)
	})
	if updateErr != nil || allocations != 0 {
		t.Fatalf("moving sparse instances: allocations=%g error=%v", allocations, updateErr)
	}
}
