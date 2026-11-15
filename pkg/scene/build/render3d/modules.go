//go:build !js

// Package render3dbuild supplies optional 3D modules to the scene compiler.
package render3dbuild

import (
	"fmt"
	"strconv"

	modelbuild "github.com/karty-game/karty-webgpu/pkg/model/build"
	scenebuild "github.com/karty-game/karty-webgpu/pkg/scene/build"
)

const (
	modulePath          = "github.com/karty-game/karty-webgpu/pkg/"
	schemaType          = "type"
	intensityProperty   = "intensity"
	maxCoordinate       = 1e8
	descriptionProperty = "description"
	stringType          = "string"
)

// Modules registers spatial transforms, prepared models and existing light components.
// All spatial composition is calculated offline; no runtime hierarchy is introduced.
func Modules() []scenebuild.Module {
	return []scenebuild.Module{transformModule(), modelModule(), ambientModule(), sunModule(), pointModule()}
}

func base(name, goType string, properties map[string]any, required ...string) scenebuild.Module {
	return scenebuild.Module{Name: name, GoType: goType, Schema: scenebuild.Object(properties, required...),
		Imports:  map[string]string{"engine": modulePath + "engine", "scene3d": modulePath + "scene/render3d"},
		Services: map[string]string{"Render": "*scene3d.Context"}}
}

func vectorSchema(description string) map[string]any {
	const vectorWidth = 3

	return map[string]any{
		schemaType: "array",
		"minItems": vectorWidth,
		"maxItems": vectorWidth,
		"items": map[string]any{
			schemaType: "number",
			"minimum":  -maxCoordinate,
			"maximum":  maxCoordinate,
		},
		descriptionProperty: description,
	}
}

func numberSchema(minimum, maximum float64) map[string]any {
	return map[string]any{schemaType: "number", "minimum": minimum, "maximum": maximum}
}

func transformModule() scenebuild.Module {
	module := base("spatial.transform3d", "engine.Transform", map[string]any{
		"position": vectorSchema("Local XYZ position in world units."),
		"rotation": vectorSchema("Local XYZ Euler rotation in radians; Rz * Ry * Rx."),
		"scale":    vectorSchema("Positive local XYZ scale; omitted means unit scale."),
	})
	module.Generate = func(component *scenebuild.Component) (string, error) {
		transform, err := transformCode(component)

		return "scene.Ready(" + transform + ")", err
	}

	return module
}

func modelModule() scenebuild.Module {
	const preparedModelSchema = modelbuild.PreparedSchema

	module := base("render3d.model", "*scene3d.ModelView", map[string]any{
		"isDynamic": map[string]any{
			schemaType:          "boolean",
			descriptionProperty: "Keep this placement separate from build-merged static geometry; defaults to false.",
		},
		"asset": map[string]any{
			schemaType:          stringType,
			"minLength":         1,
			descriptionProperty: "Prepared model source identity from an earlier asset processor.",
		},
	}, "asset")
	module.Resources = map[string]string{"asset": preparedModelSchema}
	module.Generate = func(component *scenebuild.Component) (string, error) {
		asset, err := component.Resource("asset")
		if err != nil {
			return "", err
		}

		transform, err := transformCode(component)

		return fmt.Sprintf("scene3d.NewModel(%s,services.Render,%s,%s)", component.Root, strconv.Quote(asset), transform), err
	}

	return module
}

func ambientModule() scenebuild.Module {
	module := base("render3d.hemisphere", "engine.Entity", map[string]any{intensityProperty: numberSchema(0, 10)})
	module.Imports["light"] = modulePath + "light"
	module.Generate = func(component *scenebuild.Component) (string, error) {
		return fmt.Sprintf(
			"scene3d.Add(%s,services.Render,engine.DefaultTransform(),light.Hemisphere{Name:%s,Direction:engine.Vector3Up(),SkyColor:engine.ColorWhite(),GroundColor:engine.Color{R:0.35,G:0.4,B:0.45,A:1},Intensity:%.9g})",
			component.Root,
			strconv.Quote(component.Path),
			component.Number(intensityProperty, 0.7),
		), nil
	}

	return module
}

func sunModule() scenebuild.Module {
	module := base("render3d.directional", "engine.Entity", map[string]any{
		intensityProperty: numberSchema(0, 10), "direction": vectorSchema("Direction light rays travel."),
	})
	module.Imports["light"] = modulePath + "light"
	module.Generate = func(component *scenebuild.Component) (string, error) {
		direction := vector(component.Config, "direction", [3]float64{-0.5, -1, -0.4})

		return fmt.Sprintf(
			"scene3d.Add(%s,services.Render,engine.DefaultTransform(),light.Directional{Name:%s,Direction:%s,Color:engine.Color{R:1,G:0.9,B:0.75,A:1},Intensity:%.9g})",
			component.Root,
			strconv.Quote(component.Path),
			vectorCode(direction),
			component.Number(intensityProperty, 1),
		), nil
	}

	return module
}

func pointModule() scenebuild.Module {
	module := base("render3d.point", "engine.Entity", map[string]any{
		intensityProperty: numberSchema(0, 100), "range": numberSchema(0.01, 1000),
		"color": vectorSchema("Linear RGB light color."), "enabled": map[string]any{schemaType: stringType, "minLength": 1},
		"bake": map[string]any{schemaType: stringType, "enum": []string{"static", "dynamic"},
			descriptionProperty: "Static contributes to level lightmaps; dynamic preserves live control. Defaults to dynamic."},
	}, "enabled")
	module.Imports["light"] = modulePath + "light"
	module.References = map[string]string{"enabled": "logic.bool"}
	module.Generate = func(component *scenebuild.Component) (string, error) {
		enabled, err := component.Reference("enabled")
		if err != nil {
			return "", err
		}

		transform, err := transformCode(component)
		color := vector(component.Config, "color", [3]float64{1, 0.6, 0.2})

		return fmt.Sprintf(
			"scene3d.Point(%s,services.Render,%s,light.Point{Name:%s,Color:engine.Color{R:%.9g,G:%.9g,B:%.9g,A:1},Intensity:%.9g,Range:%.9g},%s)",
			component.Root,
			transform,
			strconv.Quote(component.Path),
			color[0],
			color[1],
			color[2],
			component.Number(intensityProperty, 4),
			component.Number("range", 6),
			enabled,
		), err
	}

	return module
}
