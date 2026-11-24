//go:build js && wasm

package browser

import (
	"syscall/js"
	"testing"

	"github.com/karty-game/ashura/internal/impostordata"
	"github.com/karty-game/ashura/internal/render"
)

func TestDrawMaterialsBindsSharedShadowOnlyForPipelineChanges(t *testing.T) {
	t.Parallel()

	driver, calls := mixedMaterialDrawFixture()
	frame := render.Frame{}

	for _, draw := range driver.draws {
		entry := driver.materials[draw.call.ID]
		entry.state.Update(draw.call)
	}

	for range 2 {
		pass := js.Global().Get("Function").New(`return {
  pipelines: [], shadowBindings: [], draws: [],
  setPipeline(p) { this.pipeline = p; this.pipelines.push(p); },
  setVertexBuffer() {}, setIndexBuffer() {},
  setBindGroup(slot, group) { if (slot === 1) { this.shadow = group; this.shadowBindings.push(group); } },
  drawMesh(v, i, uv, ix, f, g, count, instances) {
   this.setVertexBuffer(0,v); this.setVertexBuffer(1,i);
   if (uv !== undefined) this.setVertexBuffer(2,uv);
   this.setIndexBuffer(ix,f); this.setBindGroup(0,g); this.drawIndexed(count,instances);
  },
  drawIndexed(...args) { this.draws.push({ args, pipeline: this.pipeline, shadow: this.shadow }); }
 }`).Invoke()
		if err := driver.drawMaterials(pass, frame); err != nil {
			t.Fatal(err)
		}

		if pass.Get("pipelines").Length() != 7 || pass.Get("shadowBindings").Length() != 6 ||
			pass.Get("draws").Length() != len(calls) {
			t.Fatal("redundant shadow binds or missing pipeline/draw commands", pass)
		}

		for index, call := range calls {
			actual := pass.Get("draws").Index(index)

			args := actual.Get("args")
			if args.Length() != 2 || args.Index(0).Int() != 3 || args.Index(1).Int() != index+1 ||
				!actual.Get("pipeline").Equal(driver.drawPipeline(call)) {
				t.Fatal("draw lost its index/instance count or selected the wrong pipeline", index, actual)
			}

			if call.Impostor == nil {
				variant := 0
				if call.Source {
					variant += 2
				}

				if call.CullBackfaces {
					variant++
				}

				if !actual.Get("shadow").Equal(driver.pointShadow.sampleGroups[variant]) {
					t.Fatal("pipeline change retained an incompatible shadow group", index, actual)
				}
			}
		}
	}
}

func mixedMaterialDrawFixture() (*Driver, []render.DrawCall) {
	config := &impostordata.Document{}
	calls := []render.DrawCall{
		{}, {}, {CullBackfaces: true}, {CullBackfaces: true},
		{Source: true, CullBackfaces: true}, {Source: true, CullBackfaces: true},
		{Source: true}, {Source: true}, {Impostor: config}, {Impostor: config},
		{Source: true}, {},
	}

	driver := &Driver{
		standardPipeline: js.ValueOf("standard"), standardCulledPipeline: js.ValueOf("standard culled"),
		sourcePipeline: js.ValueOf("Source"), sourceCulledPipeline: js.ValueOf("Source culled"),
		impostorPipeline: js.ValueOf("impostor"), indexFormat: js.ValueOf("uint32"),
		pointShadow: &gpuPointShadow{}, materials: make(map[uint32]*gpuMaterial),
	}
	for variant := range 4 {
		driver.pointShadow.sampleGroups[variant] = js.ValueOf(variant + 1)
	}

	for index := range calls {
		call := &calls[index]
		call.ID = uint32(index + 1)
		driver.materials[call.ID] = &gpuMaterial{
			source: call.Source, culled: call.CullBackfaces, config: call.Impostor,
			group: js.ValueOf(index + 1),
		}
		driver.draws = append(driver.draws, browserDraw{call: *call, count: 3, instances: index + 1, instance: &gpuInstances{}})
	}

	return driver, calls
}
