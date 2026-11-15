//go:build !js

package scenebuild

import (
	"encoding/json"
	"fmt"
	"maps"
)

// Resolved is a build-only, privately owned snapshot of a validated expanded scene.
// It retains logical identities independently of render-resource compilation.
type Resolved struct {
	Source string         `json:"source"`
	Nodes  []ResolvedNode `json:"nodes"`
}

// MaxResolvedBytes bounds expanded snapshots including repeated ancestor data.
const MaxResolvedBytes = 64 << 20

// ResolvedReference is the concrete target of a typed component property.
type ResolvedReference struct {
	Path      string `json:"path"`
	Component string `json:"component"`
}

// ResolvedNode contains the expanded component values and resolved typed edges.
// All maps and slices are owned by this result and may be edited by its consumer.
type ResolvedNode struct {
	Path       string                                  `json:"path"`
	Source     string                                  `json:"source"`
	SaveID     string                                  `json:"saveId"`
	Components map[string]map[string]any               `json:"components"`
	Ancestors  []map[string]map[string]any             `json:"ancestors"`
	Bindings   map[string]string                       `json:"bindings"`
	References map[string]map[string]ResolvedReference `json:"references"`
}

// Resolve expands prefabs, checks schemas/resources/references and runs the same
// module semantic validation as Compile. read must confine project-relative paths.
// Generators execute as trusted build code; no generated constructor is executed.
func Resolve(source string, modules []Module, resources map[string]string,
	read func(string) ([]byte, error),
) (Resolved, error) {
	build, err := resolve(source, modules, resources, read)
	if err != nil {
		return Resolved{}, err
	}

	if _, err = build.generate(source, "resolved", "Resolved"); err != nil {
		return Resolved{}, err
	}

	result := Resolved{Source: source, Nodes: make([]ResolvedNode, 0, len(build.nodes))}
	for _, node := range build.nodes {
		item := ResolvedNode{Path: node.path, Source: node.source, SaveID: node.saveID,
			Components: node.components, Ancestors: node.ancestors, Bindings: node.bindings,
			References: make(map[string]map[string]ResolvedReference)}
		for _, name := range sortedKeys(node.components) {
			entry := build.entries[node.path+"#"+name]
			if len(entry.dependencies) == 0 {
				continue
			}

			refs := make(map[string]ResolvedReference, len(entry.dependencies))
			for property, target := range entry.dependencies {
				refs[property] = ResolvedReference{Path: target.node.path, Component: target.module.Name}
			}

			item.References[name] = refs
		}

		result.Nodes = append(result.Nodes, item)
	}

	if err = boundResolved(result); err != nil {
		return Resolved{}, err
	}

	// Copy the complete graph in one pass: prefab ancestor/config objects may
	// otherwise alias sibling instances or the compiler's resource dictionaries.
	encoded, err := json.Marshal(result)
	if err != nil {
		return Resolved{}, err
	}

	var owned Resolved
	if err = json.Unmarshal(encoded, &owned); err != nil {
		return Resolved{}, err
	}

	return owned, nil
}

func resolve(source string, modules []Module, resources map[string]string,
	read func(string) ([]byte, error),
) (*compiler, error) {
	if read == nil {
		return nil, ErrInvalid
	}

	schema, err := resolveSchema(modules)
	if err != nil {
		return nil, fmt.Errorf("scene module schema: %w", err)
	}

	build := &compiler{read: read, schema: schema, modules: make(map[string]Module),
		resources: maps.Clone(resources), documents: make(map[string]document),
		byPath: make(map[string]*expandedNode), entries: make(map[string]*entry)}
	for _, module := range modules {
		build.modules[module.Name] = module
	}

	if err = build.expandFile(source, "", node{}, nil, nil, nil); err != nil {
		return nil, err
	}

	if err = build.connect(); err != nil {
		return nil, err
	}

	return build, nil
}
