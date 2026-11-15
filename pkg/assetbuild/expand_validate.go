//go:build !js

package assetbuild

import "fmt"

// Check the complete newly discovered batch before any of its actions can run.
// Existing predecessors were already admitted, but forward dependencies may
// connect them to new actions, so traversal follows both sets.
func validateExpansionCycles(all, added []Action) error {
	graph := make(map[string]Action, len(all))
	for _, action := range all {
		graph[action.ID] = action
	}

	state := make(map[string]uint8, len(all))

	var visit func(string) error

	visit = func(identity string) error {
		const (
			visiting = 1
			done     = 2
		)

		if state[identity] == visiting {
			return fmt.Errorf("cyclic discovered asset action %q: %w", identity, errPlan)
		}

		if state[identity] == done {
			return nil
		}

		state[identity] = visiting
		for _, ref := range graph[identity].Inputs {
			if err := visit(ref.Action); err != nil {
				return err
			}
		}

		state[identity] = done

		return nil
	}

	for _, action := range added {
		if err := visit(action.ID); err != nil {
			return err
		}
	}

	return nil
}
