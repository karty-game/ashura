//go:build ignore

package main

import (
	"github.com/karty-game/ashura/pkg/assetbuild"
	levelbuild "github.com/karty-game/ashura/pkg/level/build"
	bake "github.com/karty-game/ashura/pkg/lightmap/bake"
	meshbuild "github.com/karty-game/ashura/pkg/mesh/build"
	modelbuild "github.com/karty-game/ashura/pkg/model/build"
)

func main() {
	assetbuild.Main(func(assetbuild.Environment) assetbuild.Config {
		return assetbuild.Config{
			Output: "generated",
			Pipelines: []assetbuild.Pipeline{{
				Name: "world", Input: []string{"assets/sponza.glb", "assets/relief/*.png", "sponza.scene.yaml"},
				Import: []assetbuild.Plugin{
					modelbuild.PreparedGLB(),
					meshbuild.Cleanup(),
					assetbuild.Images(),
					levelbuild.Scenes(levelbuild.SceneOptions{Input: []string{"sponza.scene.yaml"}, SeparateMasked: true}),
				},
				Process: []assetbuild.Plugin{
					assetbuild.Resize(assetbuild.ResizeOptions{Include: []string{"*-height"}, MaxDimension: 256}),
					assetbuild.SSBump(assetbuild.SSBumpOptions{Include: []string{"*-height"}, HeightScale: 3, Rays: 128, Distance: 16}),
					assetbuild.Mipmaps(),
					levelbuild.Materials(reliefBindings()),
					levelbuild.StaticChunks(levelbuild.ChunkOptions{
						Size:   [3]float32{32, 32, 32},
						Origin: [3]float32{-16, -2, -8},
					}),
					levelbuild.LightmapUVs(
						levelbuild.UVOptions{
							Production:               true,
							TexelsPerUnit:            8,
							MaxPageSize:              512,
							Padding:                  4,
							PreserveGeometricNormals: true,
						},
					),
					meshbuild.Optimize(),
					levelbuild.ProductionBake(lightingSettings()),
				},
				Output: []assetbuild.Plugin{
					rnmPublication(),
				},
			}},
		}
	})
}

// Publish only RNM, with the same portable codec settings for both GPU families.
func rnmPublication() assetbuild.Plugin {
	quality := 0
	profile := func(variant, family string) levelbuild.PublishOptions {
		return levelbuild.PublishOptions{
			Variant: variant, Representation: "rnm-fit-v1", MaxAtlasSize: 1024, BundleChunks: true,
			NativeTextures: &levelbuild.NativeTextureOptions{
				Family: family, NativeAlbedo: true, ProfileMaterials: true,
				Encoder: assetbuild.BlockCompressionOptions{LDRQuality: &quality, SSBumpQuality: &quality},
			},
			AlbedoETC1S: &levelbuild.ETC1SOptions{Quality: 192},
		}
	}

	return levelbuild.PublishVariants(profile("RNMBC", "bc"), profile("RNMMobile", "etc2"))
}

// Authored material bindings are separate from the processing sequence.
func reliefBindings() levelbuild.MaterialOptions {
	return levelbuild.MaterialOptions{
		SSBump: map[string]map[string]string{"assets/sponza.glb": {
			"Sponza/node0/Opaque architecture and fabrics/primitive0":  "vase-height",
			"Sponza/node0/Opaque architecture and fabrics/primitive1":  "lion-height",
			"Sponza/node0/Opaque architecture and fabrics/primitive2":  "background-height",
			"Sponza/node0/Opaque architecture and fabrics/primitive3":  "floor-height",
			"Sponza/node0/Opaque architecture and fabrics/primitive5":  "arch-height",
			"Sponza/node0/Opaque architecture and fabrics/primitive6":  "column_a-height",
			"Sponza/node0/Opaque architecture and fabrics/primitive7":  "column_c-height",
			"Sponza/node0/Opaque architecture and fabrics/primitive8":  "column_b-height",
			"Sponza/node0/Opaque architecture and fabrics/primitive13": "vase_round-height",
			"Sponza/node0/Opaque architecture and fabrics/primitive14": "bricks-height",
		}},
	}
}

// This sample's selected quality preset: 32 samples, three bounces, strong smoothing.
func lightingSettings() levelbuild.ProductionOptions {
	return levelbuild.ProductionOptions{
		Worker: bake.WorkerOptions{MemoryBytes: 512 << 20},
		Sampling: bake.Options{
			Outputs: bake.OutputRNM,
			Samples: 32,
			Bounces: 3,
			Seed:    29,
			// Compensated roulette keeps the three-bounce expectation while
			// spending fewer rays on low-throughput continuation paths.
			RouletteDepth: 1,
		},
		Reconstruction: &bake.ReconstructionOptions{
			Radius:         .25,
			PlaneTolerance: .025,
			NormalCos:      .95,
			TangentCos:     .95,
			NoiseScale:     128,
			Strength:       1,
		},
		SurfaceEdges: bake.SurfaceEdgeOptions{
			MaximumCandidates: 500000000,
			MaximumEdges:      64000000,
			Ambiguity:         bake.SurfaceIsolateAmbiguity,
		},
		Verification: &bake.VerificationPolicy{
			Samples:           256,
			MaximumReceivers:  128,
			AbsoluteTolerance: .03,
			RelativeTolerance: .1,
			Confidence:        2,
		},
		Preview:     true,
		Environment: &bake.Environment{Sky: bake.Vec3{.12, .14, .18}, Ground: bake.Vec3{.025, .02, .015}},
	}
}
