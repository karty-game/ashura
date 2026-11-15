//go:build !js

// Package msdfcodec prepares the fixed Go Regular profile with an embedded
// msdfgen library. It is used only by offline asset builds.
//
//nolint:mnd // Fixed font profile and pinned wasm32 ABI; requests are bounded before narrowing.
package msdfcodec

import (
	"context"
	"crypto/sha256"
	_ "embed"
	"errors"
	"fmt"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
)

const Identity = "msdfgen-wasm/1.0.0/goregular-host1"

var errInvalid = errors.New("invalid offline MSDF request or result")

//go:embed third_party/msdfgen.wasm
var libraryCode []byte

type glyphRequest struct {
	id, width, height uint32
	left, bottom      int
}

type guest struct {
	//nolint:containedctx // Request-local view, discarded before asset action returns.
	ctx     context.Context
	module  api.Module
	scratch uint32
}

func rasterize(ctx context.Context, font []byte, requests []glyphRequest) (pixels [][]byte, err error) {
	if ctx == nil || !validRequests(font, requests) {
		return nil, errInvalid
	}

	if err = ctx.Err(); err != nil {
		return nil, err
	}

	defer func() {
		if failure := recover(); failure != nil {
			if cause, ok := failure.(error); ok {
				err = cause
			} else {
				err = fmt.Errorf("msdf guest: %v: %w", failure, errInvalid)
			}
		}

		if ctx.Err() != nil {
			err = ctx.Err()
		}

		if err != nil {
			pixels = nil
		}
	}()

	runtime := wazero.NewRuntimeWithConfig(ctx, wazero.NewRuntimeConfigCompiler().
		WithMemoryLimitPages(4096).WithCloseOnContextDone(true))
	defer func() { err = errors.Join(err, runtime.Close(context.WithoutCancel(ctx))) }()

	module, err := instantiate(ctx, runtime)
	if err != nil {
		return nil, err
	}

	return renderRequests(ctx, module, font, requests), nil
}

func renderRequests(ctx context.Context, module api.Module, font []byte, requests []glyphRequest) [][]byte {
	guest := guest{ctx: ctx, module: module}
	guest.call("__wasm_call_ctors")

	address := guest.copy(font)
	if guest.call("loadFont", uint64(address), uint64(len(font))) != 0 {
		panic(errInvalid)
	}

	// FreeType retains the supplied font bytes for the instance's lifetime.
	guest.scratch = guest.allocate(256)

	pixels := make([][]byte, 0, len(requests))
	for _, request := range requests {
		if err := ctx.Err(); err != nil {
			panic(err)
		}

		pixels = append(pixels, guest.render(request))
	}

	return pixels
}

func instantiate(ctx context.Context, runtime wazero.Runtime) (api.Module, error) {
	if fmt.Sprintf("%x", sha256.Sum256(libraryCode)) != "68ebde09f4e9109583ba3c4477de6eeecffeb56916ce8a84850c82caae510848" {
		return nil, errInvalid
	}

	compiled, err := runtime.CompileModule(ctx, libraryCode)
	if err != nil {
		return nil, err
	}
	defer func() { _ = compiled.Close(context.WithoutCancel(ctx)) }()

	if err = instantiateImports(ctx, runtime, compiled); err != nil {
		return nil, err
	}

	return runtime.InstantiateModule(ctx, compiled, wazero.NewModuleConfig().WithName("").WithStartFunctions())
}

func validRequests(font []byte, requests []glyphRequest) bool {
	if len(font) == 0 || len(font) > 16<<20 || len(requests) > 65535 {
		return false
	}

	var bytes uint64

	for _, request := range requests {
		if request.width > 512 || request.height > 512 || (request.width == 0) != (request.height == 0) ||
			request.left < -(1<<20) || request.left > 1<<20 || request.bottom < -(1<<20) || request.bottom > 1<<20 {
			return false
		}

		bytes += uint64(request.width) * uint64(request.height) * 4
	}

	return bytes <= 64<<20
}
