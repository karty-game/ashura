//go:build !js

package scenebuild_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/fs"
	"strings"
	"testing"

	"github.com/google/jsonschema-go/jsonschema"
	scenebuild "github.com/karty-game/karty-webgpu/pkg/scene/build"
	logicbuild "github.com/karty-game/karty-webgpu/pkg/scene/build/logic"
)

func compile(t *testing.T, files map[string]string, entry string, modules []scenebuild.Module) ([]byte, error) {
	t.Helper()

	return scenebuild.Compile(entry, "fixture", modules, nil, func(name string) ([]byte, error) {
		value, found := files[name]
		if !found {
			return nil, fmt.Errorf("missing fixture %q: %w", name, fs.ErrNotExist)
		}

		return []byte(value), nil
	})
}

func mirrorModule() scenebuild.Module {
	return scenebuild.Module{Name: "test.mirror", GoType: "*scene.Value[bool]",
		Schema:     scenebuild.Object(map[string]any{"source": map[string]any{"type": "string"}}, "source"),
		References: map[string]string{"source": "logic.bool"},
		Generate: func(component *scenebuild.Component) (string, error) {
			ref, err := component.Reference("source")

			return "scene.Ready(" + ref + ")", err
		}}
}

func TestStrictAuthoringRejectsAmbiguityAndBadReferences(t *testing.T) {
	t.Parallel()

	modules := append(logicbuild.Modules(), mirrorModule())

	cases := map[string]string{
		"unknown root":          "version: 1\nroot: {id: a, mystery: true}",
		"unknown module":        "version: 1\nroot: {id: a, components: {unknown: {}}}",
		"unknown property":      "version: 1\nroot: {id: a, components: {logic.bool: {initial: false, typo: true}}}",
		"string bool":           "version: 1\nroot: {id: a, components: {logic.bool: {initial: 'true'}}}",
		"duplicate":             "version: 1\nroot: {id: a, id: b}",
		"anchor":                "version: 1\nroot: &root {id: a}",
		"alias":                 "version: 1\nroot: {id: a, children: [*root]}",
		"tag":                   "version: 1\nroot: {id: !!str a}",
		"merge":                 "version: 1\nroot: {id: a, <<: {id: b}}",
		"null":                  "version: 1\nroot: {id: a, children: null}",
		"octal":                 "version: 01\nroot: {id: a}",
		"nonfinite":             "version: 1\nroot: {id: a, components: {logic.bool: {initial: .nan}}}",
		"multidoc":              "version: 1\nroot: {id: a}\n---\nversion: 1\nroot: {id: b}",
		"duplicate node":        "version: 1\nroot: {id: a, children: [{id: b}, {id: b}]}",
		"duplicate save id":     "version: 1\nroot: {id: a, children: [{id: b, save_id: same}, {id: c, save_id: same}]}",
		"missing ref":           "version: 1\nroot: {id: a, components: {test.mirror: {source: /missing}}}",
		"missing save identity": "version: 1\nroot: {id: a, components: {logic.bool: {persistent: true}}}",
		"absolute source":       "version: 1\nroot: {id: a, instance: /etc/secret.scene.yaml}",
	}
	for name, source := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			if _, err := compile(t, map[string]string{"root.scene.yaml": source}, "root.scene.yaml", modules); err == nil {
				t.Fatal("invalid scene accepted")
			}
		})
	}
}

func TestRootPrefabForwardsInputsWithoutChangingTheirType(t *testing.T) {
	t.Parallel()

	modules := append(logicbuild.Modules(), mirrorModule())

	files := map[string]string{
		"world.scene.yaml": `version: 1
root:
  id: world
  children:
    - id: lamp
      instance: wrapper.scene.yaml
      inputs: {power: /power}
    - id: power
      components:
        logic.bool: {initial: true}
`,
		"wrapper.scene.yaml": `version: 1
inputs: {power: logic.bool}
root:
  id: wrapper
  instance: lamp.scene.yaml
  inputs: {power: $power}
`,
		"lamp.scene.yaml": `version: 1
inputs: {power: logic.bool}
root:
  id: lamp
  components:
    test.mirror: {source: $power}
`,
	}
	if _, err := compile(t, files, "world.scene.yaml", modules); err != nil {
		t.Fatal(err)
	}

	files["lamp.scene.yaml"] = strings.Replace(files["lamp.scene.yaml"], "power: logic.bool", "power: test.mirror", 1)
	if _, err := compile(t, files, "world.scene.yaml", modules); err == nil {
		t.Fatal("forwarding changed a declared input type")
	}
}

func TestPrefabForwardingDeterministicOutputAndSchemaParity(t *testing.T) {
	t.Parallel()

	modules := append(logicbuild.Modules(), mirrorModule())
	files := map[string]string{
		"world.scene.yaml": `version: 1
root:
  id: world
  children:
    - id: lamp
      instance: lamp.scene.yaml
      inputs: {power: /power}
    - id: power
      save_id: stable-power
      components:
        logic.bool: {initial: true, persistent: true}
`,
		"lamp.scene.yaml": `version: 1
inputs: {power: logic.bool}
root:
  id: lamp
  components:
    test.mirror: {source: $power}
`,
	}

	first, err := compile(t, files, "world.scene.yaml", modules)
	if err != nil {
		t.Fatal(err)
	}

	second, err := compile(t, files, "world.scene.yaml", modules)
	if err != nil || !bytes.Equal(first, second) {
		t.Fatalf("nondeterministic output: %v", err)
	}

	if strings.Index(string(first), "result.PowerLogicBool, err =") > strings.Index(string(first), "result.LampTestMirror, err =") {
		t.Fatal("factory did not respect typed input dependency")
	}

	data, err := scenebuild.Schema(modules)
	if err != nil {
		t.Fatal(err)
	}

	var schema jsonschema.Schema
	if err := json.Unmarshal(data, &schema); err != nil {
		t.Fatal(err)
	}

	resolved, err := schema.Resolve(nil)
	if err != nil {
		t.Fatal(err)
	}

	if err := resolved.Validate(
		map[string]any{
			"version": float64(1),
			"root":    map[string]any{"id": "a", "components": map[string]any{"logic.bool": map[string]any{"initial": "false"}}},
		},
	); err == nil {
		t.Fatal("IDE schema accepted a type rejected by the compiler")
	}
}

func TestRecursivePrefabAndConstructorCyclesFail(t *testing.T) {
	t.Parallel()

	files := map[string]string{
		"a.scene.yaml": "version: 1\nroot: {id: a, instance: b.scene.yaml}",
		"b.scene.yaml": "version: 1\nroot: {id: b, instance: a.scene.yaml}",
	}
	if _, err := compile(t, files, "a.scene.yaml", nil); err == nil {
		t.Fatal("recursive files accepted")
	}

	module := mirrorModule()
	module.References = map[string]string{"source": "test.mirror"}

	files = map[string]string{"a.scene.yaml": "version: 1\nroot: {id: a, components: {test.mirror: {source: /}}}"}
	if _, err := compile(t, files, "a.scene.yaml", []scenebuild.Module{module}); err == nil {
		t.Fatal("constructor cycle accepted")
	}
}

func TestResourceReferencesRequireTheDeclaredPreparedSchema(t *testing.T) {
	t.Parallel()

	module := scenebuild.Module{Name: "test.resource", GoType: "string",
		Schema:    scenebuild.Object(map[string]any{"asset": map[string]any{"type": "string"}}, "asset"),
		Resources: map[string]string{"asset": "test.model@1"},
		Generate: func(component *scenebuild.Component) (string, error) {
			asset, err := component.Resource("asset")

			return fmt.Sprintf("scene.Ready(%q)", asset), err
		}}

	read := func(string) ([]byte, error) {
		return []byte("version: 1\nroot: {id: root, components: {test.resource: {asset: house}}}"), nil
	}
	for _, schema := range []string{"test.model@1", "test.audio@1", ""} {
		_, err := scenebuild.Compile("world.scene.yaml", "fixture", []scenebuild.Module{module}, map[string]string{"house": schema}, read)
		if (err == nil) != (schema == "test.model@1") {
			t.Fatalf("schema %q: %v", schema, err)
		}
	}
}
