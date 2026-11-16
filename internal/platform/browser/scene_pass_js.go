//go:build js && wasm

package browser

import (
	"syscall/js"

	"github.com/karty-game/karty-webgpu/internal/render"
)

func (driver *Driver) drawScene(encoder, view js.Value, frame render.Frame) error {
	if len(frame.Draws) == 0 {
		driver.clearSurface(encoder, view, frame.ClearColor)

		return nil
	}

	pass, err := driver.beginScenePass(encoder, view, frame.ClearColor)
	if err != nil {
		return err
	}

	if err := driver.drawMaterials(pass, frame); err != nil {
		pass.Call("end")

		return err
	}

	driver.drawPointShadowDebug(pass, frame)
	pass.Call("end")

	return nil
}

// Empty worlds clear the drawable directly without allocating world depth/MSAA.
func (driver *Driver) clearSurface(encoder, view js.Value, color [4]float32) {
	if color == ([4]float32{}) {
		color[3] = 1
	}

	driver.clearValue.Set("r", float64(color[0]))
	driver.clearValue.Set("g", float64(color[1]))
	driver.clearValue.Set("b", float64(color[2]))
	driver.clearValue.Set("a", float64(color[3]))
	driver.colorAttach.Set(viewProperty, view)
	driver.colorAttach.Set("resolveTarget", js.Undefined())
	driver.colorAttach.Set(storeOpProperty, driver.storeOperation)
	pass := driver.startRenderPass(encoder, driver.clearPassDescriptor)
	pass.Call("end")
}
