//go:build js && wasm

package browser

import (
	"errors"
	"syscall/js"
	"testing"

	"github.com/karty-game/karty-webgpu/internal/impostordata"
	"github.com/karty-game/karty-webgpu/internal/platform"
	"github.com/karty-game/karty-webgpu/internal/render"
)

func TestMaterialFeaturesRegisterOnlyTheirOwnShader(t *testing.T) {
	t.Parallel()

	for _, fixture := range []struct {
		name      string
		install   func(any) error
		code      string
		pipelines int
		draw      render.DrawCall
	}{
		{"standard", RegisterStandard, render.StandardShader(), 2, render.DrawCall{}},
		{"Source", RegisterSource, render.SourceShader(), 2, render.DrawCall{Source: true}},
		{"impostor", RegisterImpostors, render.ImpostorShader(), 1, render.DrawCall{Impostor: &impostordata.Document{}}},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			t.Parallel()

			factory := js.Global().Get("Function").New(`return {
  modules: [], pipelines: [],
  createShaderModule(desc) { this.modules.push(desc); return desc; },
  createRenderPipelineAsync(desc) { this.pipelines.push(desc); return Promise.resolve({ label: desc.label }); }
 }`)
			device := factory.Invoke()

			driver := &Driver{device: device, colorFormat: "bgra8unorm", sampleCount: 1}
			if err := driver.prepareScenePass(); err != nil {
				t.Fatal(err)
			}

			if device.Get("modules").Length() != 0 {
				t.Fatal("shared pass setup compiled an unregistered material")
			}

			if err := fixture.install(driver); err != nil {
				t.Fatal(err)
			}

			if device.Get("modules").Length() != 1 || device.Get("modules").Index(0).Get("code").String() != fixture.code ||
				device.Get("pipelines").Length() != fixture.pipelines {
				t.Fatal("registration compiled other shaders or omitted required culling variants")
			}

			if err := driver.requireMaterialRenderer(fixture.draw); err != nil {
				t.Fatal("registered material is unavailable", err)
			}

			if err := fixture.install(driver); !errors.Is(err, platform.ErrFeatureDuplicate) || device.Get("modules").Length() != 1 {
				t.Fatal("duplicate registration replaced or recompiled an owned pipeline", err)
			}
		})
	}
}

func TestMaterialFeatureAdmissionPrecedesGPUUploads(t *testing.T) {
	t.Parallel()

	driver := &Driver{}
	for _, draw := range []render.DrawCall{{}, {Source: true}, {Impostor: &impostordata.Document{}}} {
		if _, err := driver.materialGroup(draw, render.Frame{}); !errors.Is(err, platform.ErrFeatureMissing) {
			t.Fatal("missing material feature was not rejected before GPU use", err)
		}
	}

	if driver.materials != nil || driver.textures != nil || driver.impostorViews != nil {
		t.Fatal("missing material feature created residency before admission")
	}

	for _, install := range []func(any) error{RegisterStandard, RegisterSource, RegisterImpostors} {
		for _, host := range []any{nil, "foreign host", (*Driver)(nil), driver} {
			if err := install(host); !errors.Is(err, platform.ErrFeatureInvalid) {
				t.Fatal("invalid renderer host admitted", err)
			}
		}
	}
}
