//go:build !js

package scenebuild

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"regexp"
	"strconv"

	"github.com/google/jsonschema-go/jsonschema"
	"go.yaml.in/yaml/v3"
)

var decimal = regexp.MustCompile(`^-?(0|[1-9][0-9]*)(\.[0-9]+)?([eE][+-]?[0-9]+)?$`)

func parse(source string, data []byte, schema *jsonschema.Resolved) (document, error) {
	if len(data) > maxSourceBytes {
		return document{}, fmt.Errorf("%s exceeds source size bound: %w", source, ErrInvalid)
	}

	decoder := yaml.NewDecoder(bytes.NewReader(data))

	var tree yaml.Node
	if err := decoder.Decode(&tree); err != nil {
		return document{}, fmt.Errorf("%s: %w", source, err)
	}

	if len(tree.Content) != 1 {
		return document{}, fmt.Errorf("%s: empty scene: %w", source, ErrInvalid)
	}

	var extra yaml.Node
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return document{}, fmt.Errorf("%s: exactly one YAML document is required: %w", source, ErrInvalid)
	}

	value, err := yamlValue(source, tree.Content[0], 0)
	if err != nil {
		return document{}, err
	}

	if err := schema.Validate(value); err != nil {
		return document{}, fmt.Errorf("%s:%d:%d: %w", source, tree.Content[0].Line, tree.Content[0].Column, err)
	}

	encoded, err := json.Marshal(value)
	if err != nil {
		return document{}, err
	}

	var result document
	if err := json.Unmarshal(encoded, &result); err != nil {
		return document{}, fmt.Errorf("%s: %w", source, err)
	}

	return result, nil
}

func yamlValue(source string, node *yaml.Node, depth int) (any, error) {
	fail := func(reason string) (any, error) {
		return nil, fmt.Errorf("%s:%d:%d: %s: %w", source, node.Line, node.Column, reason, ErrInvalid)
	}
	if depth > maxDepth || node.Anchor != "" || node.Kind == yaml.AliasNode || node.Style&yaml.TaggedStyle != 0 {
		return fail("depth bound, anchors, aliases and explicit tags are not supported")
	}

	switch node.Kind {
	case yaml.MappingNode:
		result := make(map[string]any)

		for index := 0; index < len(node.Content); index += 2 {
			key := node.Content[index]
			if key.Kind != yaml.ScalarNode || key.Tag != "!!str" || key.Value == "<<" || key.Anchor != "" ||
				key.Style&yaml.TaggedStyle != 0 {
				return fail("mapping keys must be untagged strings; merge keys are forbidden")
			}

			if _, found := result[key.Value]; found {
				return fail("duplicate mapping key " + strconv.Quote(key.Value))
			}

			value, err := yamlValue(source, node.Content[index+1], depth+1)
			if err != nil {
				return nil, err
			}

			result[key.Value] = value
		}

		return result, nil
	case yaml.SequenceNode:
		result := make([]any, 0, len(node.Content))
		for _, child := range node.Content {
			value, err := yamlValue(source, child, depth+1)
			if err != nil {
				return nil, err
			}

			result = append(result, value)
		}

		return result, nil
	case yaml.ScalarNode:
		return scalarValue(source, node)
	case yaml.DocumentNode, yaml.AliasNode:
		return fail("nested documents and aliases are forbidden")
	default:
		return fail("unsupported YAML node")
	}
}

func scalarValue(source string, node *yaml.Node) (any, error) {
	switch node.Tag {
	case "!!str":
		return node.Value, nil
	case "!!bool":
		if node.Value == "true" || node.Value == "false" {
			return node.Value == "true", nil
		}
	case "!!int", "!!float":
		if decimal.MatchString(node.Value) {
			value, err := strconv.ParseFloat(node.Value, 64)
			if err == nil && !math.IsInf(value, 0) && !math.IsNaN(value) {
				return value, nil
			}
		}
	}

	return nil, fmt.Errorf("%s:%d:%d: expected string, true/false or finite decimal number: %w", source, node.Line, node.Column, ErrInvalid)
}
