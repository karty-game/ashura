//go:build !js

package scenebuild

import (
	"encoding/json"
	"fmt"
	"go/token"
	"slices"

	"github.com/google/jsonschema-go/jsonschema"
)

const (
	schemaObject            = "object"
	schemaType              = "type"
	additionalPropertiesKey = "additionalProperties"
	maxStringLength         = 1024
	maxIdentifierLength     = 96
)

// Schema returns the same assembled schema used by the compiler, for IDE association.
func Schema(modules []Module) ([]byte, error) {
	known := make(map[string]any)
	for _, module := range modules {
		if module.Name == "" || module.Schema == nil || module.Generate == nil || module.GoType == "" || known[module.Name] != nil {
			return nil, fmt.Errorf("invalid or duplicate module %q: %w", module.Name, ErrInvalid)
		}

		known[module.Name] = module.Schema
	}

	identifier := map[string]any{schemaType: "string", "pattern": "^[A-Za-z][A-Za-z0-9_-]*$", "maxLength": maxIdentifierLength}
	stringType := map[string]any{schemaType: "string", "minLength": 1, "maxLength": maxStringLength}
	nodeSchema := Object(map[string]any{
		"id": identifier, "save_id": identifier, "instance": stringType,
		"inputs":     map[string]any{schemaType: schemaObject, additionalPropertiesKey: stringType},
		"components": Object(known),
		"children":   map[string]any{schemaType: "array", "maxItems": maxNodes, "items": map[string]any{"$ref": "#/$defs/node"}},
	}, "id")
	result := Object(map[string]any{
		"version": map[string]any{schemaType: "integer", "const": 1},
		"inputs":  map[string]any{schemaType: schemaObject, additionalPropertiesKey: stringType},
		"root":    map[string]any{"$ref": "#/$defs/node"},
	}, "version", "root")
	result["$schema"] = "https://json-schema.org/draft/2020-12/schema"
	result["title"] = "Ashura scene/prefab v1"
	result["description"] = "Build-only scene composition. Logical component state belongs to the root, independently of render residency."
	result["$defs"] = map[string]any{"node": nodeSchema}

	return json.MarshalIndent(result, "", "  ")
}

func resolveSchema(modules []Module) (*jsonschema.Resolved, error) {
	data, err := Schema(modules)
	if err != nil {
		return nil, err
	}

	var schema jsonschema.Schema
	if err := json.Unmarshal(data, &schema); err != nil {
		return nil, err
	}

	return schema.Resolve(nil)
}

func sortedKeys[T any](values map[string]T) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}

	slices.Sort(keys)

	return keys
}

func validPackage(name string) bool { return token.IsIdentifier(name) && name != "_" }
