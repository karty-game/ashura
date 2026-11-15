//go:build !js

package scenebuild_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"

	logicbuild "github.com/karty-game/karty-webgpu/pkg/scene/build/logic"
)

func TestGeneratedFactoriesExecuteLifecycleAndTypedWiring(t *testing.T) {
	t.Parallel()

	files := map[string]string{
		"world.scene.yaml": `version: 1
root:
  id: world
  children:
    - id: lamp
      instance: lamp.scene.yaml
      inputs: {power: /power}
    - id: power
      save_id: power
      components:
        logic.bool: {initial: false, persistent: true}
`,
		"lamp.scene.yaml": `version: 1
inputs: {power: logic.bool}
root:
  id: lamp
  components:
    test.mirror: {source: $power}
`,
	}

	code, err := compile(t, files, "world.scene.yaml", append(logicbuild.Modules(), mirrorModule()))
	if err != nil {
		t.Fatal(err)
	}

	directory := t.TempDir()

	moduleRoot, err := filepath.Abs("../../..")
	if err != nil {
		t.Fatal(err)
	}

	contents := map[string][]byte{
		"world_gen.go": code,
		"go.mod": []byte(
			"module scene-fixture\n\ngo 1.27.0\nrequire github.com/karty-game/karty-webgpu v0.0.0\nreplace github.com/karty-game/karty-webgpu => " + strconv.Quote(
				moduleRoot,
			) + "\n",
		),
		"world_test.go": []byte(`package fixture
import("errors";"strings";"testing";"github.com/karty-game/karty-webgpu/pkg/scene")
func TestFactory(t *testing.T){
 started,ended:=0,0
 value,err:=NewWorld("world",WorldServices{Saves:scene.NewSaves()},func(refs *World)(func(),error){started++; return func(){ended++},nil})
 if err!=nil{t.Fatal(err)}
 if value.PowerLogicBool!=value.LampTestMirror{t.Fatal("typed reference was not connected")}
 if err:=value.PowerLogicBool.Set(true);err!=nil||!value.LampTestMirror.Get(){t.Fatal("shared component state failed")}
 if started!=1||ended!=0{t.Fatal(started,ended)}
 value.Root.Close();value.Root.Close()
 if started!=1||ended!=1{t.Fatal(started,ended)}
}
func TestSetupFailureRollsBack(t *testing.T){
 saves:=scene.NewSaves();ended:=0;var captured *World
 failure:=errors.New("setup rejected")
 result,err:=NewWorld("failed",WorldServices{Saves:saves},func(refs *World)(func(),error){
   captured=refs
   if err:=refs.PowerLogicBool.Set(true);err!=nil{t.Fatal(err)}
   return func(){ended++},failure
 })
 if result!=nil||!errors.Is(err,failure)||ended!=1||!captured.Root.Closed(){t.Fatal(result,err,ended)}
 data,err:=saves.Export()
 if err!=nil||strings.Contains(string(data),"power#logic.bool"){t.Fatal(string(data),err)}
 retry,err:=NewWorld("failed",WorldServices{Saves:saves},nil)
 if err!=nil||retry.PowerLogicBool.Get(){t.Fatal("aborted construction leaked state",err)}
 retry.Root.Close()
}
`),
	}
	for name, data := range contents {
		if err := os.WriteFile(filepath.Join(directory, name), data, 0o600); err != nil {
			t.Fatal(err)
		}
	}

	command := exec.CommandContext(t.Context(), "go", "test", "-buildvcs=false", "-count=1", ".")
	command.Dir = directory

	command.Env = append(os.Environ(), "GOWORK=off")
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("generated code failed to execute: %v\n%s", err, output)
	}
}
