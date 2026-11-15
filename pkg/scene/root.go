// Package scene owns logical scene lifetimes independently of rendering resources.
// Roots, components and signals belong to the application's update goroutine.
package scene

import (
	"errors"
	"fmt"
)

var (
	// ErrClosed means the logical root has already ended.
	ErrClosed = errors.New("scene root is closed")
	// ErrIdentity means a root or component identity is empty or duplicated.
	ErrIdentity = errors.New("invalid or duplicate scene identity")
)

// Root owns logical components and their cleanup. Resource eviction never closes it.
type Root struct {
	identity string
	closed   bool
	aborted  bool
	cleanups []func()
	ids      map[string]bool
}

// NewRoot starts an explicit logical lifetime. IDs distinguish saved root instances.
func NewRoot(identity string) (*Root, error) {
	if identity == "" {
		return nil, ErrIdentity
	}

	return &Root{identity: identity, ids: make(map[string]bool)}, nil
}

// ID returns the application's durable root instance identity.
func (root *Root) ID() string { return root.identity }

// Closed reports whether the logical root has ended.
func (root *Root) Closed() bool { return root == nil || root.closed }

// Claim reserves a component's durable identity within this root.
func (root *Root) Claim(identity string) error {
	if root.Closed() {
		return ErrClosed
	}

	if identity == "" || root.ids[identity] {
		return fmt.Errorf("%q: %w", identity, ErrIdentity)
	}

	root.ids[identity] = true

	return nil
}

// Own registers cleanup in dependency order. Close invokes it in reverse order.
func (root *Root) Own(cleanup func()) error {
	if root.Closed() {
		return ErrClosed
	}

	if cleanup != nil {
		root.cleanups = append(root.cleanups, cleanup)
	}

	return nil
}

// Close ends gameplay lifetime once. It does not imply writing a save to disk.
func (root *Root) Close() {
	if root.Closed() {
		return
	}

	root.closed = true
	callbacks := root.cleanups
	root.cleanups = nil
	root.ids = nil

	for _, cleanup := range callbacks {
		defer cleanup()
	}
}

// Abort rolls back construction without retaining new save records.
func (root *Root) Abort() {
	if root.Closed() {
		return
	}

	root.aborted = true
	root.Close()
}

// Ready adapts a plain constructor value to the generated component factory ABI.
func Ready[T any](value T) (T, error) { return value, nil }
