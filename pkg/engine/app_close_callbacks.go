package engine

import (
	"errors"
	"math"
)

var (
	errNilCloseCallback   = errors.New("engine: close callback is nil")
	errCloseCallbackLimit = errors.New("engine: close callback lifetime limit reached")
)

// OnClose registers resource cleanup and returns an idempotent cancellation
// function. Callbacks run once, in unspecified order, after Closed becomes true
// and before owned resource teardown. Callbacks and cancellation belong to the
// App loop goroutine. Cancel when explicitly unloading a resource before Close.
func (app *App) OnClose(cleanup func()) (func(), error) {
	if app.Closed() {
		return nil, errAppClosed
	}

	if cleanup == nil {
		return nil, errNilCloseCallback
	}

	if app.nextCloseCallback == math.MaxUint64 {
		return nil, errCloseCallbackLimit
	}

	if app.closeCallbacks == nil {
		app.closeCallbacks = make(map[uint64]func())
	}

	app.nextCloseCallback++
	identifier := app.nextCloseCallback
	app.closeCallbacks[identifier] = cleanup

	return func() { delete(app.closeCallbacks, identifier) }, nil
}

func (app *App) runCloseCallbacks() {
	for identifier, cleanup := range app.closeCallbacks {
		delete(app.closeCallbacks, identifier)
		cleanup()
	}

	app.closeCallbacks = nil
}
