//go:build !js

// Package meshopt owns the pinned offline meshoptimizer library. Games never import it.
//

package meshopt

import (
	"context"
	"crypto/sha256"
	_ "embed"
	"errors"
	"fmt"

	"github.com/karty-game/ashura/internal/meshdata"
	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
)

const Identity = "meshoptimizer/1.0/cache-fetch@1"

var ErrInvalid = errors.New("invalid offline mesh optimization")

//go:embed third_party/encoder.wasm
var code []byte

// Session is owned by one sequential offline action; Close releases its VM.
type Session struct {
	runtime wazero.Runtime
	module  api.Module
}

// Order retains corner order and maps old vertex indices into the final buffer.
type Order struct {
	Indices  []uint32
	Remap    []uint32
	Vertices int
}

func Open(ctx context.Context) (*Session, error) {
	if ctx == nil {
		return nil, ErrInvalid
	}

	if err := ctx.Err(); err != nil {
		return nil, err
	}

	if fmt.Sprintf("%x", sha256.Sum256(code)) != "57881d86081c08140e6547561d5f43217e94d875f02b22cd89aa36f9d6a3b70a" {
		return nil, ErrInvalid
	}

	runtime := wazero.NewRuntimeWithConfig(ctx, wazero.NewRuntimeConfig().WithMemoryLimitPages(4096).WithCloseOnContextDone(true))

	module, err := runtime.InstantiateWithConfig(ctx, code, wazero.NewModuleConfig().WithStartFunctions("__wasm_call_ctors"))
	if err != nil {
		_ = runtime.Close(context.WithoutCancel(ctx))

		return nil, err
	}

	return &Session{runtime: runtime, module: module}, nil
}

func (session *Session) Close(ctx context.Context) error {
	if session == nil || session.runtime == nil {
		return nil
	}

	err := session.runtime.Close(context.WithoutCancel(ctx))
	session.module, session.runtime = nil, nil

	return err
}

func (session *Session) Reorder(ctx context.Context, indices []uint32, vertices int) (order Order, err error) {
	if ctx == nil || session == nil || session.module == nil || vertices <= 0 || vertices > meshdata.MaxVertices || len(indices) == 0 ||
		len(indices)%3 != 0 ||
		len(indices) > meshdata.MaxIndices {
		return Order{}, ErrInvalid
	}

	for offset, index := range indices {
		if index >= uint32(vertices) {
			return Order{}, ErrInvalid
		}

		if offset%4096 == 0 {
			if err := ctx.Err(); err != nil {
				return Order{}, err
			}
		}
	}

	defer func() {
		if failure := recover(); failure != nil {
			order = Order{}

			if cause, valid := failure.(error); valid {
				err = cause
			} else {
				err = fmt.Errorf("meshopt guest: %v: %w", failure, ErrInvalid)
			}
		}

		if ctx.Err() != nil {
			err = ctx.Err()
		}
	}()

	return session.execute(ctx, indices, vertices), nil
}
