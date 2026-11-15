//go:build !js

//nolint:mnd,gosec // Fixed font profile and pinned wasm32 ABI; requests are bounded before narrowing.
package msdfcodec

import (
	"encoding/binary"
	"math"
)

func (guest guest) call(name string, arguments ...uint64) uint32 {
	function := guest.module.ExportedFunction(name)
	if function == nil {
		panic(errInvalid)
	}

	values, err := function.Call(guest.ctx, arguments...)
	if err != nil {
		panic(err)
	}

	if len(values) == 0 {
		return 0
	}

	return uint32(values[0])
}

func (guest guest) allocate(size uint32) uint32 {
	if size == 0 || size > 64<<20 {
		panic(errInvalid)
	}

	address := guest.call("malloc", uint64(size))
	if address == 0 {
		panic(errInvalid)
	}

	return address
}

func (guest guest) copy(data []byte) uint32 {
	if len(data) > 64<<20 {
		panic(errInvalid)
	}

	address := guest.allocate(uint32(len(data)))
	if !guest.module.Memory().Write(address, data) {
		panic(errInvalid)
	}

	return address
}

func (guest guest) render(request glyphRequest) []byte {
	if request.width == 0 {
		return nil
	}

	if guest.call("loadGlyph", uint64(request.id), uint64(guest.scratch), 0) != 0 {
		panic(errInvalid)
	}

	shape, readable := guest.module.Memory().ReadUint32Le(guest.scratch)
	if !readable || shape == 0 {
		panic(errInvalid)
	}
	defer guest.call("destroyGlyph", uint64(shape))

	config := []float64{fontSize, -float64(request.left) / fontSize, -float64(request.bottom) / fontSize,
		6.0 / fontSize, 1, 3, float64(request.width), float64(request.height), 0}
	for index, value := range config {
		if !guest.module.Memory().WriteUint64Le(guest.scratch+128+uint32(index)*8, math.Float64bits(value)) {
			panic(errInvalid)
		}
	}

	bitmap := guest.call("generateMSDF", uint64(shape), uint64(guest.scratch+128))
	if bitmap == 0 {
		panic(errInvalid)
	}
	defer guest.call("destroyBitmap", uint64(bitmap))

	address := guest.call("getBitmapPixels", uint64(bitmap))

	data, ok := guest.module.Memory().Read(address, request.width*request.height*12)
	if !ok {
		panic(errInvalid)
	}

	return rgbaPixels(data, request.width, request.height)
}

func rgbaPixels(data []byte, width, height uint32) []byte {
	pixels := make([]byte, width*height*4)
	for row := range height {
		for column := range width {
			destination := (row*width + column) * 4

			source := ((height-1-row)*width + column) * 12
			for channel := range uint32(3) {
				value := float64(math.Float32frombits(binary.LittleEndian.Uint32(data[source+channel*4:])))
				if math.IsNaN(value) {
					panic(errInvalid)
				}

				pixels[destination+channel] = byte(max(0, min(255, math.Floor(value*255+.5))))
			}

			pixels[destination+3] = 255
		}
	}

	return pixels
}
