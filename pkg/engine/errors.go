package engine

import "errors"

var (
	// ErrClosed means an application or resource lifetime has ended.
	ErrClosed = errors.New("engine: app is closed")
	// ErrInUse means resource release must wait for its users to be removed.
	ErrInUse = errors.New("engine: resource is still in use")
	// ErrOwner means a resource is unloaded or does not belong to this App.
	ErrOwner = errors.New("engine: resource is unloaded or belongs to another app")
	// ErrPhase means an operation is unavailable in the current application phase.
	ErrPhase = errors.New("engine: operation is unavailable in this phase")
	// ErrInvalidEntity means an entity is no longer alive or belongs to another World.
	ErrInvalidEntity = errors.New("entity is not alive in this world")
	// ErrInvalidComponent means a component value is nil, including a typed nil pointer.
	ErrInvalidComponent = errors.New("engine: component is nil")
)
