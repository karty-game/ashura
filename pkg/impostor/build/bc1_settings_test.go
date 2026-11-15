//go:build !js

package impostorbuild_test

import (
	"strings"
	"testing"

	"github.com/karty-game/karty-webgpu/pkg/assetbuild"
	impostorbuild "github.com/karty-game/karty-webgpu/pkg/impostor/build"
)

func TestBC1AtlasAlignmentRejectsBeforePlanOrModelAdmission(t *testing.T) {
	t.Parallel()

	for _, options := range []impostorbuild.Options{
		{Mode: impostorbuild.Octahedral, Resolution: 16, FrameGrid: 7, Padding: 1, MipLevels: 1},
		{Mode: impostorbuild.Angular, Resolution: 16, Azimuths: 3, Elevations: 2, Padding: 1, MipLevels: 1},
		{Mode: impostorbuild.Angular, Resolution: 16, Azimuths: 2, Elevations: 3, Padding: 1, MipLevels: 1},
	} {
		// Empty plans contain no model or output actions. Misalignment must be
		// rejected here, rather than after deferred model rasterization/encoding.
		plan := &assetbuild.Plan{}
		if err := impostorbuild.Cards(options).Plan(plan); err == nil || !strings.Contains(err.Error(), "BC1 impostor atlas dimensions") {
			t.Fatal("plan admitted an impossible BC1 atlas", options, err)
		}

		// This is deliberately not a model container. The settings error must
		// precede model parsing, rasterization, packing and codec initialization.
		invalidModel := []byte("model admission must not be reached")
		if _, err := impostorbuild.Bake(t.Context(), invalidModel, options); err == nil ||
			!strings.Contains(err.Error(), "BC1 impostor atlas dimensions") {
			t.Fatal("Bake parsed the model before validating its atlas dimensions", options, err)
		}

		if _, _, err := impostorbuild.BakeWithReport(t.Context(), invalidModel, options); err == nil ||
			!strings.Contains(err.Error(), "BC1 impostor atlas dimensions") {
			t.Fatal("BakeWithReport parsed the model before validating its atlas dimensions", options, err)
		}
	}
}

func TestBC1AtlasAlignmentPreservesLayeredAndSelectedSampleSettings(t *testing.T) {
	t.Parallel()

	for _, options := range []impostorbuild.Options{
		{Mode: impostorbuild.Layered},
		{Mode: impostorbuild.Octahedral, Resolution: 128, FrameGrid: 7, Padding: 8, MipLevels: 3},
		{},
	} {
		if err := impostorbuild.Cards(options).Plan(&assetbuild.Plan{}); err != nil {
			t.Fatal("valid capture settings were rejected", options, err)
		}
	}
}
