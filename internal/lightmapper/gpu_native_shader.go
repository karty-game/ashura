//go:build !js

package lightmapper

import "embed"

const nativeShaderMaximum = 8 << 20

//go:embed wgsl/*.wgsl
var nativeGPUShaders embed.FS

func nativeGPUShaderSource() ([]byte, error) {
	entries, err := nativeGPUShaders.ReadDir("wgsl")
	if err != nil {
		return nil, err
	}

	var source []byte

	for _, entry := range entries {
		data, err := nativeGPUShaders.ReadFile("wgsl/" + entry.Name())
		if err != nil {
			return nil, err
		}

		source = append(source, data...)
		source = append(source, '\n')
	}

	if len(source) == 0 || len(source) > nativeShaderMaximum {
		return nil, ErrBounds
	}

	return source, nil
}
