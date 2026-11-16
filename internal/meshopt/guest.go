//go:build !js

//nolint:gosec // Mesh bounds and the 256MiB VM limit bound every wasm32 pointer/count cast.
package meshopt

import (
	"context"
	"encoding/binary"
	"math"
)

func (session *Session) call(ctx context.Context, name string, args ...uint64) uint64 {
	function := session.module.ExportedFunction(name)
	if function == nil {
		panic(ErrInvalid)
	}

	values, err := function.Call(ctx, args...)
	if err != nil {
		panic(err)
	}

	if len(values) == 0 {
		return 0
	}

	return values[0]
}

func (session *Session) read(address, size uint64) []byte {
	if address > math.MaxUint32 || size > 128<<20 {
		panic(ErrInvalid)
	}

	data, valid := session.module.Memory().Read(uint32(address), uint32(size))
	if !valid {
		panic(ErrInvalid)
	}

	return data
}

func (session *Session) execute(ctx context.Context, indices []uint32, vertices int) Order {
	base := session.call(ctx, "sbrk", uint64(len(indices)*4))
	remap := session.call(ctx, "sbrk", uint64(vertices*4))

	buffer := session.read(base, uint64(len(indices)*4))
	for index, value := range indices {
		binary.LittleEndian.PutUint32(buffer[index*4:], value)
	}

	session.call(ctx, "meshopt_optimizeVertexCache", base, base, uint64(len(indices)), uint64(vertices))

	count := session.call(ctx, "meshopt_optimizeVertexFetchRemap", remap, base, uint64(len(indices)), uint64(vertices))
	if count == 0 || count > uint64(vertices) {
		panic(ErrInvalid)
	}

	order := session.readOrder(base, remap, vertices, len(indices), int(count))

	end := session.call(ctx, "sbrk", 0)
	if end < base || end-base > math.MaxInt32 {
		panic(ErrInvalid)
	}

	session.call(ctx, "sbrk", uint64(uint32(int32(int64(base)-int64(end)))))

	return order
}

func (session *Session) readOrder(base, remap uint64, vertices, indices, count int) Order {
	order := Order{Indices: make([]uint32, indices), Remap: make([]uint32, vertices), Vertices: count}
	buffer := session.read(remap, uint64(vertices*4))
	seen := make([]bool, count)

	for original := range order.Remap {
		index := binary.LittleEndian.Uint32(buffer[original*4:])

		order.Remap[original] = index
		if index == math.MaxUint32 {
			continue
		}

		if index >= uint32(count) || seen[index] {
			panic(ErrInvalid)
		}

		seen[index] = true
	}

	for _, present := range seen {
		if !present {
			panic(ErrInvalid)
		}
	}

	buffer = session.read(base, uint64(indices*4))
	for offset := range order.Indices {
		original := binary.LittleEndian.Uint32(buffer[offset*4:])
		if original >= uint32(vertices) {
			panic(ErrInvalid)
		}

		index := order.Remap[original]
		if index >= uint32(count) {
			panic(ErrInvalid)
		}

		order.Indices[offset] = index
	}

	return order
}
