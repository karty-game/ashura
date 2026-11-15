//go:build !js

//nolint:mnd,gosec // Fixed font profile and pinned wasm32 ABI; requests are bounded before narrowing.
package msdfcodec

import (
	"bytes"
	"context"
	"strings"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/experimental/table"
)

func instantiateImports(ctx context.Context, runtime wazero.Runtime, compiled wazero.CompiledModule) error {
	builders := map[string]wazero.HostModuleBuilder{
		"env":                    runtime.NewHostModuleBuilder("env"),
		"wasi_snapshot_preview1": runtime.NewHostModuleBuilder("wasi_snapshot_preview1"),
	}

	for _, definition := range compiled.ImportedFunctions() {
		module, name, _ := definition.Import()

		builder, exists := builders[module]
		if !exists {
			return errInvalid
		}

		parameters, results := definition.ParamTypes(), definition.ResultTypes()
		builder.NewFunctionBuilder().WithGoModuleFunction(api.GoModuleFunc(
			func(ctx context.Context, module api.Module, stack []uint64) {
				hostCall(ctx, module, name, parameters, results, stack)
			}), parameters, results).Export(name)
	}

	for _, name := range []string{"env", "wasi_snapshot_preview1"} {
		if _, err := builders[name].Instantiate(ctx); err != nil {
			return err
		}
	}

	return nil
}

func hostCall(ctx context.Context, module api.Module, name string, parameters, results []api.ValueType, stack []uint64) {
	if err := ctx.Err(); err != nil {
		panic(err)
	}

	if strings.HasPrefix(name, "invoke_") {
		function := table.LookupFunction(module, 0, uint32(stack[0]), parameters[1:], results)

		values, err := function.Call(ctx, stack[1:len(parameters)]...)
		if err != nil {
			panic(err)
		}

		copy(stack, values)

		return
	}

	switch name {
	case "emscripten_resize_heap":
		stack[0] = growMemory(module, uint32(stack[0]))
	case "emscripten_memcpy_big":
		if stack[2] > 64<<20 {
			panic(errInvalid)
		}

		data, ok := module.Memory().Read(uint32(stack[1]), uint32(stack[2]))
		if !ok || !module.Memory().Write(uint32(stack[0]), bytes.Clone(data)) {
			panic(errInvalid)
		}
	case "environ_sizes_get":
		if !module.Memory().WriteUint32Le(uint32(stack[0]), 0) || !module.Memory().WriteUint32Le(uint32(stack[1]), 0) {
			panic(errInvalid)
		}

		stack[0] = 0
	case "environ_get":
		stack[0] = 0
	case "fd_close", "fd_write", "fd_read", "fd_seek":
		// Emscripten's split-i64 fd_seek ABI differs from WASI preview1. All
		// descriptors are denied; no filesystem, environment or network is exposed.
		stack[0] = 8
	case "__syscall_fcntl64", "__syscall_openat", "__syscall_fstat64", "__syscall_stat64", "__syscall_newfstatat", "_mmap_js", "_munmap_js":
		stack[0] = 0xffffffcc // Emscripten ENOSYS (-52), represented as wasm32.
	default:
		// Abort, C++ throws and longjmp terminate this request. Never pretend an
		// exception was handled or return partial output from a failed instance.
		panic(errInvalid)
	}
}

func growMemory(module api.Module, requested uint32) uint64 {
	if requested <= module.Memory().Size() {
		return 1
	}

	const page = 65536

	pages := (uint64(requested) - uint64(module.Memory().Size()) + page - 1) / page
	if pages > 4096 {
		return 0
	}

	if _, ok := module.Memory().Grow(uint32(pages)); ok {
		return 1
	}

	return 0
}
