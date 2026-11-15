//go:build !js

// Package logicbuild supplies optional logical-state modules to the scene compiler.
package logicbuild

import (
	"fmt"
	"strconv"

	scenebuild "github.com/karty-game/karty-webgpu/pkg/scene/build"
)

// Modules provides explicit observable boolean state, with optional persistence.
func Modules() []scenebuild.Module {
	return []scenebuild.Module{{
		Name: "logic.bool", GoType: "*scene.Value[bool]",
		Schema: scenebuild.Object(map[string]any{
			"initial": map[string]any{"type": "boolean", "description": "Initial value; a saved value takes precedence."},
			"persistent": map[string]any{
				"type":        "boolean",
				"description": "Retain across root unload and include in save snapshots. Requires save_id.",
			},
		}),
		Services: map[string]string{"Saves": "*scene.Saves"},
		Generate: func(component *scenebuild.Component) (string, error) {
			identity := component.SaveID
			if identity == "" {
				if component.Bool("persistent", false) {
					return "", fmt.Errorf("persistent state at %s requires an explicit save_id: %w", component.Path, scenebuild.ErrInvalid)
				}

				identity = component.Path
			}

			return fmt.Sprintf("scene.Bool(%s,services.Saves,%s,%t,%t)", component.Root,
				strconv.Quote(identity+"#logic.bool"), component.Bool("initial", false), component.Bool("persistent", false)), nil
		},
	}}
}
