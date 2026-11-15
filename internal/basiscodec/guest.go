//go:build !js

//nolint:gosec // Every wasm32 pointer and buffer size is admitted before narrowing.
package basiscodec

import (
	"bytes"
	"context"
	"math"

	"github.com/tetratelabs/wazero/api"
)

type guest struct {
	//nolint:containedctx // Request-local view; never stored by the build session.
	ctx    context.Context
	module api.Module
	prefix string
}

func (guest guest) call(name string, arguments ...uint64) uint64 {
	function := guest.module.ExportedFunction(guest.prefix + name)
	if function == nil {
		panic(ErrInvalid)
	}

	values, err := function.Call(guest.ctx, arguments...)
	if err != nil {
		panic(err)
	}

	if len(values) == 0 {
		return 0
	}

	return values[0]
}

func (guest guest) allocate(size uint64) uint64 {
	if size == 0 || size > 128<<20 {
		panic(ErrInvalid)
	}

	address := guest.call("alloc", size)
	if address == 0 || address > math.MaxUint32 {
		panic(ErrInvalid)
	}

	return address
}

func (guest guest) copy(data []byte) uint64 {
	address := guest.allocate(uint64(len(data)))
	if !guest.module.Memory().Write(uint32(address), data) {
		panic(ErrInvalid)
	}

	return address
}

func (guest guest) read(address, size uint64) []byte {
	if address > math.MaxUint32 || size > 128<<20 {
		panic(ErrInvalid)
	}

	data, ok := guest.module.Memory().Read(uint32(address), uint32(size))
	if !ok {
		panic(ErrInvalid)
	}

	return bytes.Clone(data)
}

func (guest guest) release(name string, arguments ...uint64) {
	if !guest.module.IsClosed() {
		guest.call(name, arguments...)
	}
}
