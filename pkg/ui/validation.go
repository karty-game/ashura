package ui

import "fmt"

// ValidationError identifies an invalid UI value. Value contains the rejected
// scalar or its byte count, never a copy of a caller's text or image payload.
// Use errors.Is(err, ErrBounds) for the category and errors.As for these details.
type ValidationError struct {
	Op     string
	Field  string
	Value  any
	Reason string
}

func (err *ValidationError) Error() string {
	return fmt.Sprintf("ui: %s %s=%v: %s", err.Op, err.Field, err.Value, err.Reason)
}

// Unwrap preserves ErrBounds matching through diagnostic and mount context.
func (err *ValidationError) Unwrap() error { return ErrBounds }

func invalid(op, field string, value any, reason string) error {
	return &ValidationError{Op: op, Field: field, Value: value, Reason: reason}
}
