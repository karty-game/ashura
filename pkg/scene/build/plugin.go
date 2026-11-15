//go:build !js

package scenebuild

import (
	"fmt"
	"path"
	"strings"

	"github.com/karty-game/karty-webgpu/pkg/assetbuild"
)

const (
	scenePackage = "scenes"
	filesSlot    = "files"
)

type plugin struct{ options Options }

// Scenes compiles selected entry scenes after their prepared resource producers.
// It publishes a scenes/ Go package and the exact assembled IDE schema.
func Scenes(options Options) assetbuild.Plugin { return plugin{options: options} }

func (plugin) Name() string { return scenePackage }

func (plugin plugin) Plan(plan *assetbuild.Plan) error {
	options := plugin.options
	if options.Package == "" {
		options.Package = scenePackage
	}

	if !validPackage(options.Package) {
		return ErrInvalid
	}

	schema, err := Schema(options.Modules)
	if err != nil {
		return err
	}

	if _, err := resolveSchema(options.Modules); err != nil {
		return err
	}

	if err := publishSchema(plan, options.Package, schema); err != nil {
		return err
	}

	selected := make(map[string]bool)

	for _, pattern := range options.Input {
		files, err := plan.Glob(pattern)
		if err != nil {
			return err
		}

		for _, file := range files {
			selected[file] = true
		}
	}

	if len(selected) == 0 {
		return fmt.Errorf("no scene entry points: %w", ErrInvalid)
	}

	resources := make(map[string]string)

	inputs := make([]assetbuild.Ref, 0, len(plan.Resources))
	for _, resource := range plan.Resources {
		schema, err := plan.SlotSchema(resource.Ref)
		if err != nil {
			return err
		}

		resources[resource.ID] = schema
		inputs = append(inputs, resource.Ref)
	}

	names := make(map[string]bool)

	for _, source := range sortedKeys(selected) {
		name := exported(strings.TrimSuffix(path.Base(source), ".scene.yaml"))
		if !validPackage(name) || names[name] {
			return fmt.Errorf("scene generated name %s: %w", name, ErrInvalid)
		}

		names[name] = true

		if err := addScene(plan, options, source, name, schema, resources, inputs); err != nil {
			return err
		}
	}

	return nil
}

func publishSchema(plan *assetbuild.Plan, packageName string, schema []byte) error {
	refs, err := plan.Add(assetbuild.Action{
		ID: "scenes/schema/" + packageName, Processor: "scenes-schema", Revision: "scene-schema@1", Options: string(schema),
		Outputs: []assetbuild.Slot{{Name: filesSlot, Schema: "karty.files@1"}},
		Run: func(*assetbuild.Context) (assetbuild.Result, error) {
			data, err := assetbuild.EncodeBundle(assetbuild.Bundle{packageName + "/scene.schema.json": append(schema, '\n')})

			return assetbuild.Result{filesSlot: data}, err
		},
	})
	if err != nil {
		return err
	}

	plan.Publish(refs[0])

	return nil
}

func addScene(
	plan *assetbuild.Plan,
	options Options,
	source, name string,
	schema []byte,
	resources map[string]string,
	inputs []assetbuild.Ref,
) error {
	refs, err := plan.Add(assetbuild.Action{
		ID: "scenes/" + source, Processor: scenePackage, Revision: "scene-go@1",
		Options: struct {
			Source, Package, Schema string
			Resources               map[string]string
		}{source, options.Package, string(schema), resources},
		Inputs: inputs, Outputs: []assetbuild.Slot{{Name: filesSlot, Schema: "karty.files@1"}},
		Run: func(ctx *assetbuild.Context) (assetbuild.Result, error) {
			code, err := Compile(source, options.Package, options.Modules, resources, ctx.ReadFile)
			if err != nil {
				return nil, err
			}

			data, err := assetbuild.EncodeBundle(assetbuild.Bundle{
				options.Package + "/" + name + "_gen.go": code,
			})

			return assetbuild.Result{filesSlot: data}, err
		},
	})
	if err != nil {
		return err
	}

	plan.Publish(refs[0])

	return nil
}
