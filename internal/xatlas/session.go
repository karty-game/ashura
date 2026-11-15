//go:build !js

package xatlas

import "context"

// Session owns the embedded portable UV library. Compiled code is shared by
// concurrent requests; each request owns its guest instance and geometry.
// Close must run after all generation requests have joined.
type Session struct{ wasm *wasiSession }

// OpenSession lazily owns the embedded library for one offline build.
func OpenSession(ctx context.Context) (*Session, error) {
	if ctx == nil {
		return nil, errFormat
	}

	if err := ctx.Err(); err != nil {
		return nil, err
	}

	return &Session{wasm: &wasiSession{}}, nil
}

// Close releases owned WASM state and invalidates the session.
func (session *Session) Close() error {
	if session == nil || session.wasm == nil {
		return nil
	}

	err := session.wasm.close()
	session.wasm = nil

	return err
}
