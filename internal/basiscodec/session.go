//go:build !js

// Package basiscodec runs pinned, offline Basis Universal libraries in Go/WASI.
// It is never imported by the game runtime.
package basiscodec

import (
	"context"
	"crypto/sha256"
	_ "embed"
	"errors"
	"fmt"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/imports/wasi_snapshot_preview1"
)

// Identity versions the exact libraries and our host-side calling contract.
const Identity = "basis-wasi/250/host1"

//go:embed third_party/encoder.wasm
var encoderCode []byte

//go:embed third_party/transcoder.wasm
var decoderCode []byte

var ErrInvalid = errors.New("invalid or unavailable offline Basis WASI operation")

// Session serializes a single build's codecs and lazily retains initialized
// instances. Close only after all callers join. Cancellation invalidates the
// affected runtime; a later request may initialize a fresh generation.
type Session struct {
	gate             chan struct{}
	runtime          wazero.Runtime
	encoder, decoder api.Module
	closed           bool
	//nolint:containedctx // Build-owned teardown must outlive an action cancellation.
	cleanup context.Context
}

func New() *Session {
	session := &Session{gate: make(chan struct{}, 1)}
	session.gate <- struct{}{}

	return session
}

func (session *Session) Close() error {
	<-session.gate
	defer func() { session.gate <- struct{}{} }()

	session.closed = true

	return session.reset()
}

func (session *Session) reset() error {
	var err error
	if session.runtime != nil {
		err = session.runtime.Close(session.cleanup)
	}

	session.runtime, session.encoder, session.decoder = nil, nil, nil

	return err
}

func (session *Session) run(ctx context.Context, operation func() error) (err error) {
	if ctx == nil || session == nil {
		return ErrInvalid
	}

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-session.gate:
	}

	defer func() { session.gate <- struct{}{} }()

	if session.closed {
		return ErrInvalid
	}

	defer func() {
		if failure := recover(); failure != nil {
			if cause, ok := failure.(error); ok {
				err = cause
			} else {
				err = fmt.Errorf("basis guest: %v: %w", failure, ErrInvalid)
			}
		}

		if err != nil {
			err = errors.Join(err, session.reset())
		}

		if ctx.Err() != nil {
			err = ctx.Err()
		}
	}()

	return operation()
}

func (session *Session) guest(ctx context.Context, encode bool) guest {
	if session.runtime == nil {
		// Retain no action context or runner snapshot for build-scoped cleanup.
		session.cleanup = context.Background()
		// One serialized build session: at most 1 GiB per module, with only an
		// encoder and transcoder retained regardless of queued asset count.
		session.runtime = wazero.NewRuntimeWithConfig(ctx, wazero.NewRuntimeConfigCompiler().
			WithMemoryLimitPages(16384).WithCloseOnContextDone(true))
		if _, err := wasi_snapshot_preview1.Instantiate(ctx, session.runtime); err != nil {
			panic(err)
		}
	}

	module, code, prefix, expected := &session.decoder, decoderCode, "bt_", "55d9480cca82d8d97c5a18eaf8e4debfbdc5667da6729eab5a85efc059e5a4e9"
	if encode {
		module, code, prefix, expected = &session.encoder, encoderCode, "bu_", "6bb6dd89c28e0bc1b639da2b897781de753d5b50e6ddbbf287212182c3a2b0b8"
	}

	if *module == nil {
		if fmt.Sprintf("%x", sha256.Sum256(code)) != expected {
			panic(ErrInvalid)
		}

		compiled, err := session.runtime.CompileModule(ctx, code)
		if err != nil {
			panic(err)
		}

		defer func() { _ = compiled.Close(context.WithoutCancel(ctx)) }()

		config := wazero.NewModuleConfig().WithName("").WithStartFunctions().WithArgs("basis-offline")

		*module, err = session.runtime.InstantiateModule(ctx, compiled, config)
		if err != nil {
			panic(err)
		}

		guest{ctx, *module, prefix}.call("init")
	}

	return guest{ctx, *module, prefix}
}
