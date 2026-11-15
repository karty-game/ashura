//go:build !js

package scenebuild

import (
	"fmt"
	"go/format"
	"maps"
	"path"
	"slices"
	"strings"

	"github.com/google/jsonschema-go/jsonschema"
)

type compiler struct {
	read      func(string) ([]byte, error)
	schema    *jsonschema.Resolved
	modules   map[string]Module
	resources map[string]string
	documents map[string]document
	nodes     []*expandedNode
	byPath    map[string]*expandedNode
	entries   map[string]*entry
	ordered   []*entry
}

// Compile emits one scene factory. read must confine all project-relative inputs.
// resources lists prepared identities admitted by the preceding asset processors.
func Compile(source, packageName string, modules []Module, resources map[string]string,
	read func(string) ([]byte, error),
) ([]byte, error) {
	if !validPackage(packageName) || read == nil {
		return nil, ErrInvalid
	}

	build, err := resolve(source, modules, resources, read)
	if err != nil {
		return nil, err
	}

	name := exported(strings.TrimSuffix(path.Base(source), ".scene.yaml"))

	code, err := build.generate(source, packageName, name)
	if err != nil {
		return nil, err
	}

	result, err := format.Source([]byte(code))
	if err != nil {
		return nil, fmt.Errorf("%s: generated Go syntax: %w", source, err)
	}

	return result, nil
}

func (build *compiler) readDocument(source string) (document, error) {
	if doc, found := build.documents[source]; found {
		return doc, nil
	}

	if len(build.documents) >= maxDocuments || !safeSource(source) {
		return document{}, fmt.Errorf("source path/document bound %q: %w", source, ErrInvalid)
	}

	data, err := build.read(source)
	if err != nil {
		return document{}, err
	}

	doc, err := parse(source, data, build.schema)
	if err != nil {
		return document{}, err
	}

	build.documents[source] = doc

	return doc, nil
}

func (build *compiler) expandFile(source, parent string, override node, bindings map[string]string,
	ancestors []map[string]map[string]any, stack []string,
) error {
	if len(stack) >= maxDepth {
		return fmt.Errorf("prefab depth: %w", ErrInvalid)
	}

	if slices.Contains(stack, source) {
		return fmt.Errorf("recursive prefab %s: %w", source, ErrInvalid)
	}

	doc, err := build.readDocument(source)
	if err != nil {
		return err
	}

	if len(doc.Inputs) != len(bindings) {
		return fmt.Errorf("%s: inputs do not match declaration: %w", source, ErrInvalid)
	}

	for name, module := range doc.Inputs {
		if bindings[name] == "" || build.modules[module].Name == "" {
			return fmt.Errorf("%s: unknown input %s/%s: %w", source, name, module, ErrInvalid)
		}

		target, inheritedType, typed := strings.Cut(bindings[name], "#")
		if typed && inheritedType != module {
			return fmt.Errorf("%s: forwarded input %s has type %s, expected %s: %w", source, name, inheritedType, module, ErrInvalid)
		}

		bindings[name] = target + "#" + module
	}

	value := doc.Root

	if override.ID != "" {
		value.ID = override.ID
	}

	if override.SaveID != "" {
		value.SaveID = override.SaveID
	}

	value.Components = maps.Clone(value.Components)
	if value.Components == nil {
		value.Components = make(map[string]map[string]any)
	}

	maps.Copy(value.Components, override.Components)
	value.Children = append(append([]node(nil), value.Children...), override.Children...)

	return build.expandNode(source, parent, value, bindings, ancestors, append(stack, source))
}

func (build *compiler) expandNode(source, parent string, value node, bindings map[string]string,
	ancestors []map[string]map[string]any, stack []string,
) error {
	if len(ancestors) > maxDepth || len(build.nodes) >= maxNodes {
		return fmt.Errorf("%s: expanded node/depth bound: %w", source, ErrInvalid)
	}

	current := "/"
	if parent != "" {
		current = path.Join(parent, value.ID)
	}

	if value.Instance != "" {
		inputBindings, err := instanceBindings(value.Inputs, bindings, current)
		if err != nil {
			return err
		}

		if path.IsAbs(value.Instance) || strings.ContainsAny(value.Instance, "\\\x00") {
			return ErrInvalid
		}

		instanceSource := path.Join(path.Dir(source), value.Instance)
		value.Instance = ""

		return build.expandFile(instanceSource, parent, value, inputBindings, ancestors, stack)
	}

	if len(value.Inputs) != 0 {
		return fmt.Errorf("%s: inputs require an instance: %w", source, ErrInvalid)
	}

	if build.byPath[current] != nil {
		return fmt.Errorf("%s: duplicate node %s: %w", source, current, ErrInvalid)
	}

	item := &expandedNode{path: current, source: source, saveID: value.SaveID,
		components: value.Components, ancestors: ancestors, bindings: bindings}
	build.nodes = append(build.nodes, item)
	build.byPath[current] = item

	for _, child := range value.Children {
		parents := append(append([]map[string]map[string]any(nil), ancestors...), value.Components)
		if err := build.expandNode(source, current, child, bindings, parents, stack); err != nil {
			return err
		}
	}

	return nil
}

func instanceBindings(inputs, bindings map[string]string, current string) (map[string]string, error) {
	result := make(map[string]string)

	for name, reference := range inputs {
		if input, found := strings.CutPrefix(reference, "$"); found {
			inherited := bindings[input]
			if inherited == "" {
				return nil, fmt.Errorf("missing forwarded input %s: %w", reference, ErrInvalid)
			}

			result[name] = inherited

			continue
		}

		resolved, err := referencePath(current, reference)
		if err != nil {
			return nil, err
		}

		result[name] = resolved
	}

	return result, nil
}

func safeSource(source string) bool {
	for _, char := range source {
		if char < ' ' {
			return false
		}
	}

	return source != "" && !strings.ContainsAny(source, "\\\x00") && !path.IsAbs(source) &&
		path.Clean(source) == source && source != ".." && !strings.HasPrefix(source, "../")
}

func exported(value string) string {
	var output strings.Builder

	upper := true

	for _, char := range value {
		if char >= 'a' && char <= 'z' || char >= 'A' && char <= 'Z' || char >= '0' && char <= '9' {
			if upper && char >= 'a' && char <= 'z' {
				char -= 'a' - 'A'
			}

			output.WriteRune(char)

			upper = false
		} else {
			upper = true
		}
	}

	return output.String()
}
