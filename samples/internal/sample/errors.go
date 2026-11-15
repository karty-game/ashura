// Package sample supplies the shared error policy for runnable samples.
// Application setup remains explicit in each sample's run function.
package sample

import "errors"

// Cleanup joins a deferred resource-release failure with a named run result.
// Register App cleanup first so scene resources are released before its owner.
func Cleanup(result *error, closeResource func() error) {
	if err := closeResource(); err != nil {
		*result = errors.Join(*result, err)
	}
}

// Errors retains the first unexpected error from callbacks that cannot return
// one. Use Check at the event boundary and return Err from OnUpdateError. It
// belongs to the App loop goroutine; recoverable user actions handle errors
// locally instead of adding them here.
type Errors struct{ first error }

// Check retains a failure until the sample's fallible update reports it.
func (pending *Errors) Check(err error) {
	if pending.first == nil {
		pending.first = err
	}
}

// Err returns the first callback failure, or nil while callbacks succeed.
func (pending *Errors) Err() error { return pending.first }
