//go:build !js

package levelbuild

import (
	"fmt"
	"maps"
	"path"
	"slices"
	"strings"

	"github.com/karty-game/ashura/internal/leveldata"
	"github.com/karty-game/ashura/internal/modeldata"
	"github.com/karty-game/ashura/pkg/assetbuild"
	"github.com/karty-game/ashura/pkg/lightmap"
	modelbuild "github.com/karty-game/ashura/pkg/model/build"
	scenebuild "github.com/karty-game/ashura/pkg/scene/build"
	logicbuild "github.com/karty-game/ashura/pkg/scene/build/logic"
	renderbuild "github.com/karty-game/ashura/pkg/scene/build/render3d"
)

const levelPointBias = .015

type importPlugin struct {
	options SceneOptions
}

func (importPlugin) Name() string { return "level-import" }
func (plugin importPlugin) Plan(plan *assetbuild.Plan) error {
	options := plugin.options
	if len(options.Modules) == 0 {
		options.Modules = append(logicbuild.Modules(), renderbuild.Modules()...)
	}

	schema, err := scenebuild.Schema(options.Modules)
	if err != nil {
		return err
	}

	if err := publishLevelSchema(plan, schema); err != nil {
		return err
	}

	sources, err := selectedLevelSources(plan, options.Input)
	if err != nil {
		return err
	}

	names := make(map[string]bool)

	for _, source := range sources {
		name := assetbuild.AssetName(strings.TrimSuffix(path.Base(source), ".scene.yaml"))
		if names[name] {
			return ErrInvalid
		}

		names[name] = true
	}

	return plan.Expand(
		assetbuild.Expansion{ID: "level/import/discover-models", Run: func(_ *assetbuild.Context, child *assetbuild.Plan) error {
			return plugin.planSources(child, sources, options, schema)
		}},
	)
}

func (plugin importPlugin) planSources(plan *assetbuild.Plan, sources []string, options SceneOptions, schema []byte) error {
	resources, schemas, err := levelResources(plan)
	if err != nil {
		return err
	}

	var inputs []assetbuild.Ref

	for _, resource := range plan.Resources {
		if schemas[resource.ID] == modelbuild.PreparedSchema {
			inputs = append(inputs, resource.Ref)
		}
	}

	for _, source := range sources {
		name := assetbuild.AssetName(strings.TrimSuffix(path.Base(source), ".scene.yaml"))
		if err := addLevelImport(plan, source, name, options, schema, resources, schemas, inputs); err != nil {
			return err
		}
	}

	return nil
}

func selectedLevelSources(plan *assetbuild.Plan, patterns []string) ([]string, error) {
	selected := make(map[string]bool)

	for _, pattern := range patterns {
		files, err := plan.Glob(pattern)
		if err != nil {
			return nil, err
		}

		for _, file := range files {
			selected[file] = true
		}
	}

	if len(selected) == 0 {
		return nil, fmt.Errorf("no authored level inputs: %w", ErrInvalid)
	}

	return slices.Sorted(maps.Keys(selected)), nil
}

func levelResources(plan *assetbuild.Plan) (map[string]assetbuild.Resource, map[string]string, error) {
	resources := make(map[string]assetbuild.Resource)
	schemas := make(map[string]string)

	for _, resource := range plan.Resources {
		schema, err := plan.SlotSchema(resource.Ref)
		if err != nil {
			return nil, nil, err
		}

		resources[resource.ID] = resource
		schemas[resource.ID] = schema
	}

	return resources, schemas, nil
}

func publishLevelSchema(plan *assetbuild.Plan, schema []byte) error {
	refs, err := plan.Add(
		assetbuild.Action{ID: "level/schema", Processor: "level-scene-schema", Revision: "scene-schema@1", Options: string(schema),
			Outputs: []assetbuild.Slot{{Name: filesSlot, Schema: filesSchema}}, Run: func(*assetbuild.Context) (assetbuild.Result, error) {
				encoded, err := assetbuild.EncodeBundle(assetbuild.Bundle{"scenes/scene.schema.json": append(slices.Clone(schema), '\n')})

				return assetbuild.Result{filesSlot: encoded}, err
			}},
	)
	if err != nil {
		return err
	}

	plan.Publish(refs[0])

	return nil
}

func addLevelImport(plan *assetbuild.Plan, source, name string, options SceneOptions, schema []byte,
	resources map[string]assetbuild.Resource, schemas map[string]string, inputs []assetbuild.Ref) error {
	refs, err := plan.Add(
		assetbuild.Action{
			ID:        "level/source/" + source,
			Processor: "level-import",
			Revision:  "scene-source@1",
			Inputs:    inputs,
			Options: struct {
				Schema         string
				Spaces         map[string]string
				SeparateMasked bool
			}{string(schema), options.Spaces, options.SeparateMasked},
			Outputs: []assetbuild.Slot{
				{Name: slot, Schema: sceneSourceSchema},
				{Name: filesSlot, Schema: filesSchema},
			},
			Run: func(ctx *assetbuild.Context) (assetbuild.Result, error) {
				resolved, err := scenebuild.Resolve(source, options.Modules, schemas, ctx.ReadFile)
				if err != nil {
					return nil, err
				}

				imported, err := importSceneSource(ctx, resolved, importOptions{
					Spaces: options.Spaces, SeparateMasked: options.SeparateMasked,
				}, resources, schemas)
				if err != nil {
					return nil, err
				}

				output, err := result(sceneSource{Scene: imported, SeparateMasked: options.SeparateMasked})
				if err != nil {
					return nil, err
				}

				code, err := scenebuild.Compile(source, "scenes", bakedModules(options.Modules), schemas, ctx.ReadFile)
				if err != nil {
					return nil, err
				}

				output[filesSlot], err = assetbuild.EncodeBundle(assetbuild.Bundle{"scenes/" + name + "_gen.go": code})

				return output, err
			},
		},
	)
	if err != nil {
		return err
	}

	plan.Publish(refs[1])
	plan.Resources = append(plan.Resources, assetbuild.Resource{ID: source, Ref: refs[0]})

	return nil
}

type importOptions struct {
	SSBump         map[string]map[string]string
	Spaces         map[string]string
	SeparateMasked bool
}

type levelImportState struct {
	options   importOptions
	resources map[string]assetbuild.Resource
	schemas   map[string]string
	models    map[string]assetbuild.Artifact
	textures  map[string]assetbuild.Artifact
	documents map[string]modeldata.Document
	materials map[string]map[string]assetbuild.Artifact
}

func importSceneSource(ctx *assetbuild.Context, resolved scenebuild.Resolved, options importOptions,
	resources map[string]assetbuild.Resource, schemas map[string]string) (imported, error) {
	record := imported{Source: resolved.Source}
	state := levelImportState{
		options:   options,
		resources: resources,
		schemas:   schemas,
		models:    map[string]assetbuild.Artifact{},
		textures:  map[string]assetbuild.Artifact{},
		documents: map[string]modeldata.Document{},
		materials: map[string]map[string]assetbuild.Artifact{},
	}

	nodes := make(map[string]scenebuild.ResolvedNode)
	for _, node := range resolved.Nodes {
		nodes[node.Path] = node
	}

	for _, node := range resolved.Nodes {
		transform, err := renderbuild.WorldTransform(node.Ancestors, node.Components)
		if err != nil {
			return record, err
		}

		object := leveldata.Object{ID: node.Path, SaveID: node.SaveID, Transform: transform}
		if node.Components["render3d.model"] != nil {
			if err := state.importNodeModel(ctx, node, transform, &object, &record, nodes); err != nil {
				return record, err
			}
		}

		record.Objects = append(record.Objects, object)

		if config := node.Components["render3d.hemisphere"]; config != nil {
			intensity := number(config, "intensity", .7)
			addImportedEnvironment(&record.SkyColor, intensity, [3]float32{1, 1, 1})
			addImportedEnvironment(&record.GroundColor, intensity, [3]float32{.35, .4, .45})
		}

		if point, enabled := staticNodeLight(node, nodes, transform, options.Spaces); enabled {
			record.Points = append(record.Points, point)
		}
	}

	return record, nil
}

func addImportedEnvironment(target *[3]float32, intensity float32, color [3]float32) {
	for axis := range target {
		target[axis] += intensity * color[axis]
	}
}

func (state *levelImportState) modelArtifact(ctx *assetbuild.Context, source string) (assetbuild.Artifact, error) {
	if state.schemas[source] != modelbuild.PreparedSchema {
		return assetbuild.Artifact{}, fmt.Errorf("model %s: %w", source, ErrInvalid)
	}

	if artifact, ok := state.models[source]; ok {
		return artifact, nil
	}

	data, err := ctx.Input(state.resources[source].Ref)
	if err != nil {
		return assetbuild.Artifact{}, err
	}

	document, err := modeldata.Decode(data)
	if err != nil {
		return assetbuild.Artifact{}, err
	}

	state.documents[source] = document

	artifact, err := ctx.StoreArtifact(data)
	if err != nil {
		return assetbuild.Artifact{}, err
	}

	state.models[source] = artifact

	return artifact, nil
}

func (state *levelImportState) overrides(ctx *assetbuild.Context, source string) (map[string]assetbuild.Artifact, error) {
	result := make(map[string]assetbuild.Artifact)

	for part, identity := range state.options.SSBump[source] {
		texture, ok := state.textures[identity]
		if !ok {
			resource, found := state.resources[identity]
			if !found {
				return nil, fmt.Errorf("missing SSBump %s: %w", identity, ErrInvalid)
			}

			var err error

			texture, err = readSSBump(ctx, resource, state.schemas[identity])
			if err != nil {
				return nil, err
			}

			state.textures[identity] = texture
		}

		result[part] = texture
	}

	return result, nil
}

func staticNodeLight(
	node scenebuild.ResolvedNode,
	nodes map[string]scenebuild.ResolvedNode,
	transform [16]float32,
	spaces map[string]string,
) (staticLight, bool) {
	config := node.Components["render3d.point"]
	if config == nil || config["bake"] != "static" {
		return staticLight{}, false
	}

	reference := node.References["render3d.point"]["enabled"]
	enabled := nodes[reference.Path].Components["logic.bool"]

	initial, _ := enabled["initial"].(bool)
	if !initial {
		return staticLight{}, false
	}

	return staticLight{
		Space: nodeSpace(node.Path, spaces),
		Point: lightmap.Point{
			Position: [3]float32{transform[12], transform[13], transform[14]},
			Color: vector(
				config,
				"color",
				[3]float32{1, .6, .2},
			),
			Intensity: number(config, "intensity", 4),
			Range:     number(config, "range", 6),
			Bias:      levelPointBias,
		},
	}, true
}

func nodeSpace(node string, spaces map[string]string) string {
	for {
		if name := spaces[node]; name != "" {
			return name
		}

		if node == "/" || node == "." {
			return "default"
		}

		node = path.Dir(node)
	}
}
func number(config map[string]any, key string, fallback float32) float32 {
	if value, ok := config[key].(float64); ok {
		return float32(value)
	}

	return fallback
}
func vector(config map[string]any, key string, fallback [3]float32) [3]float32 {
	if values, ok := config[key].([]any); ok && len(values) == 3 {
		for i := range fallback {
			if value, ok := values[i].(float64); ok {
				fallback[i] = float32(value)
			}
		}
	}

	return fallback
}

func bakedModules(modules []scenebuild.Module) []scenebuild.Module {
	result := slices.Clone(modules)
	for index, module := range result {
		original := module.Generate
		switch module.Name {
		case "render3d.model":
			result[index].Generate = func(component *scenebuild.Component) (string, error) {
				code, err := original(component)
				if !component.Bool("isDynamic", false) {
					code = strings.Replace(code, "scene3d.NewModel(", "scene3d.NewBakedModel(", 1)
				}

				return code, err
			}
		case "render3d.point":
			result[index].Generate = func(component *scenebuild.Component) (string, error) {
				if component.String("bake", "dynamic") == "static" {
					configured := *component
					configured.Config = maps.Clone(component.Config)
					configured.Config["intensity"] = float64(0)

					return original(&configured)
				}

				return original(component)
			}
		}
	}

	return result
}
