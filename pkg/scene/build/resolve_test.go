//go:build !js

package scenebuild_test

import (
	"fmt"
	"strings"
	"testing"

	scenebuild "github.com/karty-game/ashura/pkg/scene/build"
	logicbuild "github.com/karty-game/ashura/pkg/scene/build/logic"
)

func TestResolveExportsExpandedReferencesAndOwnedInstances(t *testing.T) {
	t.Parallel()

	files := map[string]string{
		"world.scene.yaml": `version: 1
root:
  id: world
  components:
    logic.bool: {initial: false}
  children:
    - id: power
      save_id: saved-power
      components:
        logic.bool: {initial: true, persistent: true}
    - id: first
      instance: lamp.scene.yaml
      inputs: {power: /power}
    - id: second
      instance: lamp.scene.yaml
      inputs: {power: /power}
`,
		"lamp.scene.yaml": `version: 1
inputs: {power: logic.bool}
root:
  id: lamp
  components:
    test.mirror: {source: $power}
`,
	}
	read := func(name string) ([]byte, error) { return []byte(files[name]), nil }

	modules := append(logicbuild.Modules(), mirrorModule())

	resolved, err := scenebuild.Resolve("world.scene.yaml", modules, nil, read)
	if err != nil {
		t.Fatal(err)
	}

	if len(resolved.Nodes) != 4 || resolved.Nodes[1].SaveID != "saved-power" {
		t.Fatalf("logical identities lost: %+v", resolved)
	}

	first, second := resolved.Nodes[2], resolved.Nodes[3]

	target := first.References["test.mirror"]["source"]
	if first.Path != "/first" || first.Source != "lamp.scene.yaml" || target.Path != "/power" ||
		target.Component != "logic.bool" || first.Bindings["power"] != "/power#logic.bool" || len(first.Ancestors) != 1 {
		t.Fatalf("reference/prefab provenance lost: %+v", first)
	}

	first.Components["test.mirror"]["source"] = "/changed"

	first.Ancestors[0]["logic.bool"]["initial"] = true
	if second.Components["test.mirror"]["source"] != "$power" || second.Ancestors[0]["logic.bool"]["initial"] != false {
		t.Fatal("resolved prefab instances alias mutable component/ancestor data")
	}
}

func TestResolvePreservesSemanticAndResourceAdmission(t *testing.T) {
	t.Parallel()

	badPersistent := "version: 1\nroot: {id: bad, components: {logic.bool: {persistent: true}}}"

	if _, err := scenebuild.Resolve("bad.scene.yaml", logicbuild.Modules(), nil,
		func(string) ([]byte, error) { return []byte(badPersistent), nil }); err == nil {
		t.Fatal("Resolve bypassed module semantic validation")
	}

	resource := scenebuild.Module{Name: "test.resource", GoType: "bool",
		Schema:    scenebuild.Object(map[string]any{"asset": map[string]any{"type": "string"}}, "asset"),
		Resources: map[string]string{"asset": "fixture.model@1"},
		Generate: func(component *scenebuild.Component) (string, error) {
			_, err := component.Resource("asset")

			return "scene.Ready(false)", err
		}}

	read := func(string) ([]byte, error) {
		return []byte("version: 1\nroot: {id: root, components: {test.resource: {asset: mesh}}}"), nil
	}
	if _, err := scenebuild.Resolve("root.scene.yaml", []scenebuild.Module{resource}, nil, read); err == nil {
		t.Fatal("missing prepared resource admitted")
	}

	if _, err := scenebuild.Resolve(
		"root.scene.yaml",
		[]scenebuild.Module{resource},
		map[string]string{"mesh": "fixture.model@1"},
		read,
	); err != nil {
		t.Fatal(err)
	}
}

func TestResolveBoundsRepeatedAncestorSnapshots(t *testing.T) {
	t.Parallel()

	payload := scenebuild.Module{Name: "test.payload", GoType: "bool",
		Schema:   scenebuild.Object(map[string]any{"text": map[string]any{"type": "string"}}),
		Generate: func(*scenebuild.Component) (string, error) { return "scene.Ready(false)", nil }}

	var source strings.Builder
	source.WriteString("version: 1\nroot:\n  id: root\n  components:\n    test.payload: {text: '")
	source.WriteString(strings.Repeat("x", 512<<10))
	source.WriteString("'}\n  children:\n")

	for index := range 32 {
		fmt.Fprintf(&source, "    - {id: child%d}\n", index)
	}

	if _, err := scenebuild.Resolve("root.scene.yaml", []scenebuild.Module{payload}, nil,
		func(string) ([]byte, error) { return []byte(source.String()), nil }); err == nil || !strings.Contains(err.Error(), "snapshot") {
		t.Fatalf("unbounded ancestor snapshot admission: %v", err)
	}
}
