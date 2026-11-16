//go:build js && wasm

package framehost

import (
	"runtime"
	"syscall/js"
	"testing"
	"time"
)

const (
	allocationFrames  = 128
	allocationBallast = 24 << 20
	maxFrameBytes     = 8 << 10
	maxFrameObjects   = 96
)

//nolint:paralleltest // Owns the JS factory and process-wide allocation/GC counters.
func TestPrimitiveFramesKeepAllocationAndFinalizerPressureBounded(t *testing.T) {
	previous := js.Global().Get("__ashuraCreateFrameHost")

	t.Cleanup(func() { js.Global().Set("__ashuraCreateFrameHost", previous) })
	js.Global().Get("Function").New(Source).Invoke()
	host := js.Global().Call("__ashuraCreateFrameHost")

	t.Cleanup(func() { host.Call("close") })

	fixture := js.Global().Get("Function").New(allocationGPUFixture).Invoke()
	context, device, queue := fixture.Get("context"), fixture.Get("device"), fixture.Get("queue")
	encoder, render, compute := host.Get("encoder"), host.Get("renderPass"), host.Get("computePass")
	resource, descriptor := fixture.Get("resource"), fixture.Get("descriptor")
	descriptor.Get("colorAttachments").Index(0).Set("view", host.Get("surface"))

	indexFormat := js.ValueOf("uint16")
	frame := func() {
		if !host.Call("begin", context, device).Bool() || !encoder.Call("beginComputePass").Bool() {
			t.Fatal("frame or compute pass refused")
		}

		compute.Call("setPipeline", resource)
		compute.Call("setBindGroup", 0, resource)
		compute.Call("dispatchWorkgroups", 1, 1, 1)
		compute.Call("end")

		if !encoder.Call("beginRenderPass", descriptor).Bool() {
			t.Fatal("render pass refused")
		}

		render.Call("setPipeline", resource)
		render.Call("drawMesh", resource, resource, resource, resource, indexFormat, resource, 6, 8)
		render.Call("end")

		if !host.Call("submit", queue).Bool() {
			t.Fatal("frame submission refused")
		}
	}

	ballast := make([]byte, allocationBallast)
	for offset := 0; offset < len(ballast); offset += 4096 {
		ballast[offset] = 1
	}

	for range 8 {
		frame()
	}
	// Clear setup garbage, then let any queued setup finalizers finish. Yielding
	// also reaches TinyGo's pressure-GC scheduler boundary, unlike a tight loop.
	runtime.GC()
	time.Sleep(time.Millisecond)
	runtime.GC()
	time.Sleep(time.Millisecond)

	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)

	for range allocationFrames {
		frame()
		time.Sleep(time.Nanosecond)
	}

	runtime.ReadMemStats(&after)
	runtime.KeepAlive(ballast)

	bytes, objects := after.TotalAlloc-before.TotalAlloc, after.Mallocs-before.Mallocs
	t.Logf("%d frames: %d bytes, %d objects, %d GC cycles; %.1f bytes/frame, %.1f objects/frame",
		allocationFrames, bytes, objects, after.NumGC-before.NumGC,
		float64(bytes)/allocationFrames, float64(objects)/allocationFrames)
	// Allow bounded compiler/runtime overhead (8 KiB / 96 objects) rather than
	// asserting zero while syscall/js argument slices and boxing still allocate.
	if bytes > allocationFrames*maxFrameBytes || objects > allocationFrames*maxFrameObjects {
		t.Fatalf("frame bridge exceeded bounded allocation budget: %d bytes, %d objects", bytes, objects)
	}

	if after.NumGC != before.NumGC {
		t.Fatalf("primitive frames triggered GC despite large idle heap: %d cycles", after.NumGC-before.NumGC)
	}

	if fixture.Get("submits").Int() != allocationFrames+8 || fixture.Get("draws").Int() != allocationFrames+8 ||
		fixture.Get("dispatches").Int() != allocationFrames+8 {
		t.Fatal("representative frames did not execute the expected GPU operations")
	}
}

const allocationGPUFixture = `
const resource = {}, surface = {}, state = {submits: 0, draws: 0, dispatches: 0};
class Pass {
 constructor(kind) { this.kind = kind; this.open = true; }
 check() { if (!(this instanceof Pass) || !this.open) throw Error('invalid pass receiver/lifetime'); }
 setPipeline(value) { this.check(); if (value !== resource) throw Error('pipeline changed'); }
 setVertexBuffer(slot,value) { this.check(); if(slot < 0 || slot > 2 || value !== resource) throw Error('vertex changed'); }
 setIndexBuffer(value,format) { this.check(); if(value !== resource || format !== 'uint16') throw Error('index changed'); }
 setBindGroup(index,value) { this.check(); if(index !== 0 || value !== resource) throw Error('group changed'); }
 drawIndexed(indices,instances) { this.check(); if(this.kind !== 'render' || indices !== 6 || instances !== 8) throw Error('draw changed'); state.draws++; }
 dispatchWorkgroups(x,y,z) { this.check(); if(this.kind !== 'compute' || x !== 1 || y !== 1 || z !== 1) throw Error('dispatch changed'); state.dispatches++; }
 end() { this.check(); this.open = false; }
}
class Encoder {
 beginComputePass() { if (!(this instanceof Encoder)) throw Error('encoder receiver'); return new Pass('compute'); }
 beginRenderPass(desc) { if (!(this instanceof Encoder) || desc.colorAttachments[0].view !== surface) throw Error('surface changed'); return new Pass('render'); }
 finish() { if (!(this instanceof Encoder)) throw Error('encoder receiver'); return {}; }
}
const context = {getCurrentTexture() { if (this !== context) throw Error('context receiver'); return {createView() { return surface; }}; }};
const device = {createCommandEncoder() { if (this !== device) throw Error('device receiver'); return new Encoder(); }};
const queue = {submit(commands) { if(this !== queue || commands.length !== 1 || !commands[0]) throw Error('submission changed'); state.submits++; }};
return Object.assign(state,{context,device,queue,resource,descriptor:{colorAttachments:[{view:null,loadOp:'clear',storeOp:'store'}]}});
`
