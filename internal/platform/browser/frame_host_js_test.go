//go:build js && wasm

package browser

import (
	"errors"
	"syscall/js"
	"testing"

	"github.com/karty-game/karty-webgpu/internal/platform/browser/framehost"
	"github.com/karty-game/karty-webgpu/internal/render"
)

//nolint:paralleltest // Owns the browser bootstrap factory during this test.
func TestFrameHostRequiresCompatibleBootstrap(t *testing.T) {
	previous := js.Global().Get("__ashuraCreateFrameHost")

	t.Cleanup(func() { js.Global().Set("__ashuraCreateFrameHost", previous) })
	js.Global().Set("__ashuraCreateFrameHost", js.Undefined())

	if _, err := newFrameHost(); !errors.Is(err, errFrameHost) {
		t.Fatal("missing bootstrap accepted", err)
	}

	js.Global().Set("__ashuraCreateFrameHost", js.Global().Get("Function").New("return {version: 2}"))

	if _, err := newFrameHost(); !errors.Is(err, errFrameHost) {
		t.Fatal("incompatible bootstrap accepted", err)
	}

	for _, code := range []string{"return {version: 1}", "return {version: 1.5}"} {
		js.Global().Set("__ashuraCreateFrameHost", js.Global().Get("Function").New(code))

		if _, err := newFrameHost(); !errors.Is(err, errFrameHost) {
			t.Fatal("incomplete or fractional-version bootstrap accepted", err)
		}
	}
}

//nolint:paralleltest // Installs and restores the shared browser bootstrap factory.
func TestFrameHostDriverSubmitsAndRecoversAfterFailure(t *testing.T) {
	installTestFrameHost(t)

	host, err := newFrameHost()
	if err != nil {
		t.Fatal(err)
	}

	fixture := js.Global().Get("Function").New(`
const state = {begins: 0, passes: 0, ends: 0, submits: 0, fail: false};
const view = {};
const texture = {createView() { return view; }};
state.context = {getCurrentTexture() { state.begins++; return texture; }};
state.device = {createCommandEncoder() { return {
 beginRenderPass(desc) {
  if (state.fail) throw new Error('injected pass failure');
  if (desc.colorAttachments[0].view !== view) throw new Error('surface marker leaked');
  state.passes++;
  return {end() { state.ends++; }};
 },
 finish() { return {}; }
}; }};
state.queue = {submit(commands) {
 if (commands.length !== 1 || !commands[0]) throw new Error('invalid submission');
 state.submits++;
}};
return state;`).Invoke()

	driver := &Driver{
		context: fixture.Get("context"), device: fixture.Get("device"), queue: fixture.Get("queue"),
		frameHost: host, frameEncoder: host.Get("encoder"), renderPass: host.Get("renderPass"),
		computePass: host.Get("computePass"), surfaceView: host.Get("surface"),
	}
	if err := driver.prepareScenePass(); err != nil {
		t.Fatal(err)
	}

	for range 3 {
		if err := driver.present(render.Frame{}); err != nil || driver.frameOpen {
			t.Fatal("frame failed or remained open", err)
		}
	}

	fixture.Set("fail", true)
	func() {
		defer func() {
			if recover() == nil {
				t.Error("GPU pass exception was swallowed")
			}
		}()

		_ = driver.present(render.Frame{})
	}()

	if driver.frameOpen {
		t.Fatal("failed frame retained active host state")
	}

	if !driver.passDescriptor.Get("depthStencilAttachment").Equal(driver.depthAttach) {
		t.Fatal("failed clear frame removed the scene depth attachment")
	}

	fixture.Set("fail", false)

	if err := driver.present(render.Frame{}); err != nil {
		t.Fatal("host could not recover after abort", err)
	}

	if fixture.Get("submits").Int() != 4 || fixture.Get("passes").Int() != 4 || fixture.Get("ends").Int() != 4 {
		t.Fatal("frame omitted or repeated a pass/submission", fixture)
	}
}

func installTestFrameHost(t *testing.T) {
	t.Helper()

	previous := js.Global().Get("__ashuraCreateFrameHost")

	t.Cleanup(func() { js.Global().Set("__ashuraCreateFrameHost", previous) })
	js.Global().Get("Function").New(framehost.Source).Invoke()
}
