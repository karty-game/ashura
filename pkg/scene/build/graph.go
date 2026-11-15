//go:build !js

package scenebuild

import (
	"fmt"
	"strings"
)

func (build *compiler) connect() error {
	if err := build.declareEntries(); err != nil {
		return err
	}

	for _, key := range sortedKeys(build.entries) {
		value := build.entries[key]
		for property, expected := range value.module.References {
			config, ok := value.node.components[value.module.Name][property].(string)
			if !ok {
				return fmt.Errorf("%s: reference %s requires a node path or $input: %w", key, property, ErrInvalid)
			}

			var targetKey string
			if input, found := strings.CutPrefix(config, "$"); found {
				targetKey = value.node.bindings[input]
			} else {
				resolved, err := referencePath(value.node.path, config)
				if err != nil {
					return err
				}

				targetKey = resolved + "#" + expected
			}

			target := build.entries[targetKey]
			if target == nil || target.module.Name != expected {
				return fmt.Errorf("%s: %s does not reference a %s: %w", key, config, expected, ErrInvalid)
			}

			value.dependencies[property] = target
		}
	}

	for _, key := range sortedKeys(build.entries) {
		if err := build.visit(build.entries[key]); err != nil {
			return err
		}
	}

	return nil
}

func (build *compiler) declareEntries() error {
	fields := map[string]bool{"Root": true}
	saveIdentities := make(map[string]bool)

	for _, node := range build.nodes {
		if node.saveID != "" {
			if saveIdentities[node.saveID] {
				return fmt.Errorf("duplicate root-scoped save_id %s: %w", node.saveID, ErrInvalid)
			}

			saveIdentities[node.saveID] = true
		}

		for _, name := range sortedKeys(node.components) {
			field := exported(node.path + "_" + name)
			if fields[field] || len(build.entries) >= maxComponents {
				return fmt.Errorf("generated field collision/component bound %s: %w", field, ErrInvalid)
			}

			fields[field] = true
			build.entries[node.path+"#"+name] = &entry{node: node, module: build.modules[name], field: field,
				dependencies: make(map[string]*entry)}
		}
	}

	return nil
}

func (build *compiler) visit(value *entry) error {
	if value.visiting {
		return fmt.Errorf("component construction cycle %s: %w", value.field, ErrInvalid)
	}

	if value.visited {
		return nil
	}

	value.visiting = true
	for _, property := range sortedKeys(value.dependencies) {
		if err := build.visit(value.dependencies[property]); err != nil {
			return err
		}
	}

	value.visiting, value.visited = false, true
	build.ordered = append(build.ordered, value)

	return nil
}
