//go:build js && wasm

package browser

import "syscall/js"

type gpuGeometry struct {
	vertices, indices js.Value
	lightmapUV        js.Value
}

func (geometry gpuGeometry) release() {
	geometry.vertices.Call("destroy")
	geometry.indices.Call("destroy")

	if !geometry.lightmapUV.IsUndefined() {
		geometry.lightmapUV.Call("destroy")
	}
}

func (driver *Driver) createGeometry(vertices, indices, lightmapUV []byte) (gpuGeometry, error) {
	const vertexUsage, indexUsage = 0x28, 0x18

	vertexBuffer, err := driver.geometryBuffer(vertices, vertexUsage, "mesh vertices")
	if err != nil {
		return gpuGeometry{}, err
	}

	indexBuffer, err := driver.geometryBuffer(indices, indexUsage, "mesh indices")
	if err != nil {
		vertexBuffer.Call("destroy")

		return gpuGeometry{}, err
	}

	result := gpuGeometry{vertices: vertexBuffer, indices: indexBuffer}
	if len(lightmapUV) != 0 {
		result.lightmapUV, err = driver.geometryBuffer(lightmapUV, vertexUsage, "lightmap UVs")
		if err != nil {
			result.release()

			return gpuGeometry{}, err
		}
	}

	return result, nil
}

func (driver *Driver) geometryBuffer(data []byte, usage int, label string) (js.Value, error) {
	buffer := driver.device.Call("createBuffer", jsObject(map[string]any{
		sizeProperty: len(data), usageProperty: usage, labelProperty: label,
	}))
	if buffer.IsUndefined() || buffer.IsNull() {
		return js.Undefined(), errDrawBuffer
	}

	array := js.Global().Get("Uint8Array").New(len(data))
	js.CopyBytesToJS(array, data)
	driver.queue.Call("writeBuffer", buffer, 0, array)

	return buffer, nil
}
