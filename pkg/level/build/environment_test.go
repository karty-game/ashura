//go:build !js

package levelbuild

import (
	"math"
	"testing"

	"github.com/karty-game/karty-webgpu/pkg/assetbuild"
	scenebuild "github.com/karty-game/karty-webgpu/pkg/scene/build"
)

func TestImportPreservesAuthoredHemisphereEnvironment(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name       string
		components []map[string]any
		intensity  float32
	}{
		{name: "default", components: []map[string]any{{}}, intensity: .7},
		{name: "sum", components: []map[string]any{{"intensity": float64(.4)}, {"intensity": float64(.2)}}, intensity: .6},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			resolved := scenebuild.Resolved{Source: "ambient.scene.yaml"}
			for _, component := range test.components {
				resolved.Nodes = append(
					resolved.Nodes,
					scenebuild.ResolvedNode{Components: map[string]map[string]any{"render3d.hemisphere": component}},
				)
			}

			imported, err := importResolved(&assetbuild.Context{Context: t.Context()}, resolved, ImportOptions{}, nil, nil)
			if err != nil {
				t.Fatal(err)
			}

			for axis, ground := range [3]float32{.35, .4, .45} {
				if math.Abs(float64(imported.SkyColor[axis]-test.intensity)) > 1e-6 ||
					math.Abs(float64(imported.GroundColor[axis]-test.intensity*ground)) > 1e-6 {
					t.Fatalf("authored environment sky%v ground%v", imported.SkyColor, imported.GroundColor)
				}
			}
		})
	}
}
