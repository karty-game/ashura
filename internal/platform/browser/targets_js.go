//go:build js && wasm

package browser

import "syscall/js"

func (driver *Driver) ensureTargets() error {
	width, height := driver.canvasWidth, driver.canvasHeight
	if width < 1 || height < 1 {
		return nil
	}

	if width == driver.depthWidth && height == driver.depthHeight && !driver.depthView.IsUndefined() {
		return nil
	}

	driver.releaseTargets()

	depth, depthView, err := driver.createAttachment("depth24plus")
	if err != nil {
		return err
	}

	driver.depthTexture, driver.depthView = depth, depthView
	if driver.sampleCount > 1 {
		color, colorView, err := driver.createAttachment(driver.colorFormat)
		if err != nil {
			return err
		}

		driver.colorTexture, driver.colorView = color, colorView
	}

	driver.depthWidth, driver.depthHeight = width, height

	return nil
}

func (driver *Driver) createAttachment(format string) (js.Value, js.Value, error) {
	texture := driver.device.Call(
		"createTexture",
		jsObject(
			map[string]any{
				sizeProperty: jsObject(
					map[string]any{widthProperty: driver.canvasWidth, heightProperty: driver.canvasHeight, depthOrArrayLayersProperty: 1},
				),
				formatProperty: format,
				usageProperty:  textureUsageRenderAttachment,
				"sampleCount":  driver.sampleCount,
			},
		),
	)
	if texture.IsUndefined() || texture.IsNull() {
		return js.Undefined(), js.Undefined(), errDepthTexture
	}

	view := texture.Call("createView")
	if view.IsUndefined() || view.IsNull() {
		texture.Call("destroy")

		return js.Undefined(), js.Undefined(), errDepthView
	}

	return texture, view, nil
}

func (driver *Driver) attachColor(surface js.Value) {
	if driver.sampleCount > 1 {
		driver.colorAttach.Set(viewProperty, driver.colorView)
		driver.colorAttach.Set("resolveTarget", surface)
		driver.colorAttach.Set(storeOpProperty, driver.discardOperation)
	} else {
		driver.colorAttach.Set(viewProperty, surface)
		driver.colorAttach.Set("resolveTarget", js.Undefined())
		driver.colorAttach.Set(storeOpProperty, driver.storeOperation)
	}
}

func (driver *Driver) releaseTargets() {
	for _, texture := range []js.Value{driver.depthTexture, driver.colorTexture} {
		if !texture.IsUndefined() && !texture.IsNull() {
			texture.Call("destroy")
		}
	}

	driver.depthTexture, driver.depthView = js.Undefined(), js.Undefined()
	driver.colorTexture, driver.colorView = js.Undefined(), js.Undefined()
}
