//go:build !js

package scenebuild

import "fmt"

func boundResolved(value Resolved) error {
	remaining := MaxResolvedBytes
	for _, node := range value.Nodes {
		if !snapshotBytes(&remaining, 512+6*(len(node.Path)+len(node.Source)+len(node.SaveID))) {
			return fmt.Errorf("expanded scene snapshot exceeds %d bytes: %w", MaxResolvedBytes, ErrInvalid)
		}

		configs := append(append([]map[string]map[string]any(nil), node.Ancestors...), node.Components)
		for _, components := range configs {
			for name, config := range components {
				if !snapshotBytes(&remaining, 32+6*len(name)) || !snapshotValue(config, &remaining) {
					return fmt.Errorf("expanded scene snapshot exceeds %d bytes: %w", MaxResolvedBytes, ErrInvalid)
				}
			}
		}

		for name, binding := range node.Bindings {
			if !snapshotBytes(&remaining, 32+6*(len(name)+len(binding))) {
				return ErrInvalid
			}
		}

		for component, references := range node.References {
			if !snapshotBytes(&remaining, 32+6*len(component)) {
				return ErrInvalid
			}

			for property, target := range references {
				if !snapshotBytes(&remaining, 64+6*(len(property)+len(target.Path)+len(target.Component))) {
					return ErrInvalid
				}
			}
		}
	}

	return nil
}

func snapshotValue(value any, remaining *int) bool {
	if !snapshotBytes(remaining, 32) {
		return false
	}

	switch value := value.(type) {
	case nil, bool, float64:
		return true
	case string:
		return snapshotBytes(remaining, 6*len(value))
	case []any:
		for _, item := range value {
			if !snapshotValue(item, remaining) {
				return false
			}
		}
	case map[string]any:
		for name, item := range value {
			if !snapshotBytes(remaining, 32+6*len(name)) || !snapshotValue(item, remaining) {
				return false
			}
		}
	default:
		return false
	}

	return true
}

func snapshotBytes(remaining *int, count int) bool {
	if count < 0 || count > *remaining {
		return false
	}

	*remaining -= count

	return true
}
