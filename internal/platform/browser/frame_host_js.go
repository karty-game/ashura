//go:build js && wasm

package browser

import "syscall/js"

// The static browser bootstrap installs this factory. Current-frame GPU objects
// stay inside its bounded JavaScript storage; Go retains only these facades.
func newFrameHost() (js.Value, error) {
	factory := js.Global().Get("__ashuraCreateFrameHost")
	if factory.Type() != js.TypeFunction {
		return js.Undefined(), errFrameHost
	}

	host := factory.Invoke()
	if host.Type() != js.TypeObject || host.IsNull() {
		return js.Undefined(), errFrameHost
	}

	version := host.Get("version")
	if version.Type() != js.TypeNumber || version.Float() != 1 || !validFrameHost(host) {
		return js.Undefined(), errFrameHost
	}

	return host, nil
}

func validFrameHost(host js.Value) bool {
	return frameHostObject(host, "begin", "submit", "reset", "close") &&
		frameHostObject(host.Get("encoder"), "beginRenderPass", "beginComputePass") &&
		frameHostObject(host.Get("renderPass"), "setPipeline", "setVertexBuffer", "setIndexBuffer", "setBindGroup",
			"setScissorRect", "drawIndexed", "drawMesh", "draw", "end") &&
		frameHostObject(host.Get("computePass"), "setPipeline", "setBindGroup", "dispatchWorkgroups", "end") &&
		frameHostObject(host.Get("surface"))
}

func frameHostObject(value js.Value, methods ...string) bool {
	if value.Type() != js.TypeObject || value.IsNull() {
		return false
	}

	for _, method := range methods {
		if value.Get(method).Type() != js.TypeFunction {
			return false
		}
	}

	return true
}

func (driver *Driver) abortFrame() {
	if driver.frameOpen {
		driver.frameHost.Call("reset")
		driver.frameOpen = false
	}
}

func (driver *Driver) initFrameHost(host js.Value) {
	driver.frameHost, driver.frameEncoder = host, host.Get("encoder")
	driver.renderPass, driver.computePass = host.Get("renderPass"), host.Get("computePass")
	driver.surfaceView = host.Get("surface")
}

func (driver *Driver) startRenderPass(encoder, descriptor js.Value) js.Value {
	encoder.Call("beginRenderPass", descriptor)

	return driver.renderPass
}
