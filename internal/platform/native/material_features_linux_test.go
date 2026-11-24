//go:build linux

package native

import (
	"errors"
	"testing"

	"github.com/karty-game/ashura/internal/impostordata"
	"github.com/karty-game/ashura/internal/platform"
	"github.com/karty-game/ashura/internal/render"
)

func TestMaterialFeatureAdmissionPrecedesGPUUploads(t *testing.T) {
	t.Parallel()

	driver := &Driver{}
	for _, draw := range []render.DrawCall{{}, {Source: true}, {Impostor: &impostordata.Document{}}} {
		if _, err := driver.materialGroup(draw); !errors.Is(err, platform.ErrFeatureMissing) {
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
