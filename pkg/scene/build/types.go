//go:build !js

// Package scenebuild compiles strict, modular scene YAML into ordinary Go.
// It is a build-tool dependency; games import scene and their component modules.
package scenebuild

import (
	"errors"
	"fmt"
	"path"
	"strings"
)

const (
	maxSourceBytes = 2 << 20
	maxDocuments   = 128
	maxDepth       = 64
	maxNodes       = 8192
	maxComponents  = 32768
)

// ErrInvalid marks invalid authored composition or module contracts.
var ErrInvalid = errors.New("invalid authored scene")

// Module supplies one strictly validated component and a direct Go factory expression.
// Expressions return (GoType, error). Imports and services are explicit, not inferred.
type Module struct {
	Name       string
	GoType     string
	Schema     map[string]any
	Imports    map[string]string
	Services   map[string]string
	References map[string]string
	Resources  map[string]string
	Generate   func(*Component) (string, error)
}

// Component is the build-only context passed to a module's code generator.
type Component struct {
	Config    map[string]any
	Path      string
	SaveID    string
	Root      string
	Ancestors []map[string]map[string]any
	compiler  *compiler
	entry     *entry
}

// NodeComponents exposes schema-validated sibling configuration for offline composition.
func (component *Component) NodeComponents() map[string]map[string]any {
	return component.entry.node.components
}

// String reads a schema-validated optional string property.
func (component *Component) String(name, fallback string) string {
	value, ok := component.Config[name].(string)
	if !ok {
		return fallback
	}

	return value
}

// Number reads a schema-validated optional number property.
func (component *Component) Number(name string, fallback float64) float64 {
	value, ok := component.Config[name].(float64)
	if !ok {
		return fallback
	}

	return value
}

// Bool reads a schema-validated optional boolean property.
func (component *Component) Bool(name string, fallback bool) bool {
	value, ok := component.Config[name].(bool)
	if !ok {
		return fallback
	}

	return value
}

// Reference returns the generated typed field for a declared component dependency.
func (component *Component) Reference(property string) (string, error) {
	target := component.entry.dependencies[property]
	if target == nil {
		return "", fmt.Errorf("%s missing declared reference %s: %w", component.Path, property, ErrInvalid)
	}

	return "result." + target.field, nil
}

// Resource validates and returns a project-relative prepared resource identity.
func (component *Component) Resource(property string) (string, error) {
	name := component.String(property, "")

	expected := component.entry.module.Resources[property]
	if expected == "" || component.compiler.resources[name] != expected {
		return "", fmt.Errorf("%s: missing prepared resource %q: %w", component.Path, name, ErrInvalid)
	}

	return name, nil
}

// Object describes a strict object schema with documented module properties.
func Object(properties map[string]any, required ...string) map[string]any {
	result := map[string]any{"type": schemaObject, additionalPropertiesKey: false, "properties": properties}
	if len(required) > 0 {
		result["required"] = required
	}

	return result
}

type document struct {
	Version int               `json:"version"`
	Inputs  map[string]string `json:"inputs"`
	Root    node              `json:"root"`
}

type node struct {
	ID         string                    `json:"id"`
	SaveID     string                    `json:"save_id"` //nolint:tagliatelle // The scene source format consistently uses snake_case.
	Instance   string                    `json:"instance"`
	Inputs     map[string]string         `json:"inputs"`
	Components map[string]map[string]any `json:"components"`
	Children   []node                    `json:"children"`
}

type expandedNode struct {
	path, source, saveID string
	components           map[string]map[string]any
	ancestors            []map[string]map[string]any
	bindings             map[string]string
}

type entry struct {
	node         *expandedNode
	module       Module
	field        string
	dependencies map[string]*entry
	visited      bool
	visiting     bool
}

func referencePath(current, value string) (string, error) {
	if value == "" || strings.ContainsAny(value, "\\\x00") {
		return "", ErrInvalid
	}

	if strings.HasPrefix(value, "/") {
		return path.Clean(value), nil
	}

	joined := path.Clean(path.Join(strings.TrimPrefix(current, "/"), value))
	if joined == ".." || strings.HasPrefix(joined, "../") {
		return "", fmt.Errorf("reference escapes scene root: %w", ErrInvalid)
	}

	return "/" + strings.TrimPrefix(joined, "."), nil
}
