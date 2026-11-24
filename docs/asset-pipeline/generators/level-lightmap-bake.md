# Bake level lighting

[Asset pipeline](../README.md) · [Generators](README.md)

Use `levelbuild.ProductionBake` to calculate lighting for a static level before
shipping it. It traces light from the scene's sources and environment, including
bounced light, then stores the result in textures. The game samples those
textures instead of tracing the same lighting every frame.

Choose ordinary RGB maps for diffuse lighting, or RNM maps when normal maps and
surface relief should change the response to that light. RNM uses three maps
and needs more texture space. The [processor glossary](../processors/README.md#terms-used-in-these-guides)
explains these terms.

The baker is Go/WGSL code in Ashura using
[wgpu-native](https://github.com/gfx-rs/wgpu-native) through the existing Go
binding. It runs on the build machine's GPU in the same process as the recipe.

Import `levelbuild` from `github.com/karty-game/ashura/pkg/level/build`
and `bake` from `github.com/karty-game/ashura/pkg/lightmap/bake`.
Put this plugin in `Pipeline.Process` after [StaticChunks](../processors/level-static-chunks.md)
with `UVs` set, or after separately staged production
[LightmapUVs](../processors/level-lightmap-uvs.md) with `UVs` nil. Follow with a publisher.

To apply [Meshopt ordering](../processors/mesh-optimize.md) to the final UV geometry, use the
separate `LightmapUVs` → `meshbuild.Optimize()` → `ProductionBake` form and leave
`ProductionBake.UVs` nil. The integrated UV form has no intermediate stage for
that insertion. Optional [Cleanup](../processors/mesh-cleanup.md) belongs after prepared model
import and before scene import in either form.

## Settings and defaults

`ProductionBake(levelbuild.ProductionOptions{...})` takes:

| Setting | Default | Meaning |
| --- | --- | --- |
| `UVs *UVOptions` | `nil` | Nil consumes existing production UV inputs. Non-nil prepares them from chunks and automatically sets `Production: true`; other defaults are on the [UV page](../processors/level-lightmap-uvs.md). |
| `ResumeSamples []string` | None | Project input patterns for saved fixed-count bake results. The build checks their integrity and compatibility. Cannot be combined with `Adaptive`. |
| `Worker bake.WorkerOptions` | Memory budget unset; must be supplied | Build-time GPU configuration, detailed below. |
| `Sampling bake.Options` | RGB + RNM, 256 samples, zero bounces/seed/roulette, hierarchy light sampling | Fixed trace settings, detailed below. |
| `Adaptive *bake.AdaptivePolicy` | `nil` | Opt into independent pilot allocation and fixed production counts. |
| `Reconstruction *bake.ReconstructionOptions` | `nil` | Opt into geometry-guided filtering before final encoding. |
| `SurfaceEdges bake.SurfaceEdgeOptions` | Zero limits | Limits for finding connected surfaces to filter together when reconstruction is active. |
| `Verification *bake.VerificationPolicy` | `nil` | Separate directional reference pass. Required for final RNM. |
| `Preview bool` | `false` | Allows an explicitly marked preview; directional verification failures remain diagnostics rather than qualifying final RNM. |
| `MetresPerUnit float32` | `1` | Physical scene scale; zero selects 1. |
| `Environment *bake.Environment` | `nil` | Nil uses the scene's authored sky/ground lighting. Non-nil replaces it with explicit radiance values. |
| `Emitters map[string][]bake.Emitter` | None added | Additional emitters keyed by logical space, in addition to authored static lights. |
| `BackendIdentity string` | Empty | GPU/driver/backend cache identity. Empty disables caching of GPU results. |

Start from the preview example below and choose an explicit GPU memory budget.
`ProductionOptions{}` alone cannot run: its memory budget is unset, and its
default final RNM output also needs a verification policy.

### Worker and sampling

| Setting | Default | Meaning |
| --- | --- | --- |
| `Worker.MemoryBytes uint64` | None; required positive | GPU baking memory budget, independent of quality. Other build-process memory is separate. |
| `Worker.TraceRays bool` | `false` | Enables integer offline ray diagnostics. |
| `Worker.SoftwareForCI bool` | `false` | Explicit Vulkan CPU execution for functional CI; ordinary builds require hardware. |
| `Sampling.Outputs bake.Outputs` | `bake.OutputRGB \| bake.OutputRNM` | Requested outputs; zero selects both. Publication selects one representation without retracing. |
| `Sampling.Samples uint32` | `256` | Fixed sample count; zero selects 256; maximum `1 << 24`. |
| `Sampling.Bounces uint32` | `0` | Bounce depth; maximum 32. Zero is a real choice. |
| `Sampling.Seed uint32` | `0` | Deterministic sample seed. |
| `Sampling.RouletteDepth uint32` | `0` | Zero disables compensated roulette; positive values enable it at that depth, up to 32. |
| `Sampling.ExactLights bool` | `false` | Exhaustive reference light evaluation instead of production hierarchy sampling. |

Authored point lights remain dynamic unless their scene component explicitly
sets `bake: static`. Static lights use their authored initial enabled state;
the generated live intensity becomes zero to avoid counting light twice.

### Optional adaptive sampling and filtering

These policies have no automatically selected quality preset. Supply their
fields explicitly rather than assuming zero means a recommended value.

| Setting | Default / requirement | Meaning |
| --- | --- | --- |
| `Adaptive.PilotSamples` | Required at least 2 | Independent pilot count, excluded from production estimates. |
| `Adaptive.Quality` | Explicit | `Minimum`, `Maximum`, `AbsoluteError`, `RelativeError`, `Confidence` bound count allocation and statistical diagnostics. |
| `Adaptive.Strict` | `false` | Fail if the completed fixed production pass does not meet requested quality. |
| `Adaptive.Attempt` | Required nonempty | A name identifying this adaptive sampling attempt. |
| `Reconstruction.Radius`, `PlaneTolerance` | Explicit world units | Neighborhood size and allowed distance from the surface plane when filtering. |
| `Reconstruction.NormalCos`, `TangentCos` | Explicit | Cosine thresholds limiting how much neighboring surface orientations may differ. |
| `Reconstruction.NoiseScale`, `Strength` | Explicit | Noise-aware filtering amount; strength zero disables surface-support preparation. |
| `Reconstruction.MaximumHalo` | `0` | Uses the actual device and GPU memory limits for neighboring samples. A positive value adds a stricter sample-count limit. |
| `SurfaceEdges.MaximumCandidates`, `MaximumEdges` | Required positive for active support | Maximum potential and accepted surface connections. Exceeding either limit fails the build. |
| `SurfaceEdges.Tolerance` | `0` | Coordinate-precision tolerance, not a blur radius. |
| `SurfaceEdges.Support` | Derived by this plugin | The plugin uses its reconstruction settings to choose which surfaces may connect. |
| `SurfaceEdges.Ambiguity` | Strict rejection | `bake.SurfaceIsolateAmbiguity` explicitly isolates ambiguous contacts and retains diagnostics. |

Adaptive quality requires `Minimum >= 2`, `Maximum >= Minimum`, positive
`AbsoluteError` and `Confidence`, and nonnegative `RelativeError`. Reconstruction
requires positive `Radius` and `NoiseScale`, nonnegative `PlaneTolerance`, and
`NormalCos`, `TangentCos` and `Strength` within 0–1. Floating-point settings must
be finite.

### Directional verification and lighting

`Verification.Samples` must be at least 2; `Seed` defaults to zero.
`AbsoluteTolerance` must be positive, while `RelativeTolerance` and `Confidence`
may be zero. `MaximumReceivers: 0` checks all receivers; a positive value selects
a diagnostic subset. `RequireAllReceivers` and `RequireMaterialDomain` default to
false but are forced true for final output. `MaterialDomains` supplies actual
prepared material normal domains with material identity, domain identity and
normal samples. A preview subset does not establish final RNM quality.

An explicit `Environment` contains `Sky`, `Ground`, `Texture` and
`LegacyIrradiance`. Colors are radiance; a nonzero latitude-longitude linear
texture replaces the gradient. `LegacyIrradiance` selects the explicit divide-by-pi
adapter. See the [baker guide](../../../pkg/lightmap/bake/README.md) for physical
emitter units, environment textures and further lighting options.

## How to use it

This fragment bakes RNM previews after `StaticChunks`; the quality and memory
values are examples, not engine defaults:

```go
levelbuild.ProductionBake(levelbuild.ProductionOptions{
    UVs: &levelbuild.UVOptions{
        TexelsPerUnit: 8, MaxPageSize: 512, Padding: 4,
    },
    Worker: bake.WorkerOptions{MemoryBytes: 512 << 20},
    Sampling: bake.Options{
        Outputs: bake.OutputRNM,
        Samples: 32, Bounces: 3, Seed: 29,
    },
    Environment: &bake.Environment{
        Sky: bake.Vec3{.12, .14, .18},
        Ground: bake.Vec3{.025, .02, .015},
    },
    Preview: true,
}),
```

Use [PublishVariants](../publishers/level-publish-variants.md) for explicit RNM desktop/mobile
profiles, or [Publish](../publishers/level-publish.md) for one profile. For final RNM, supply full
verification and change `Preview` to false only after the domain passes it.

Run the recipe through `engine assets build`; it requires a supported native
GPU library/backend on the build host. If the GPU/driver changes, change
`BackendIdentity` or leave it empty for uncached GPU work. The pipeline tracks
the native library and recipe implementation as well.

[Complete Sponza recipe](../../../samples/sponza/asset.config.go) ·
[Plugin source](../../../pkg/level/build/production.go) · [Pipeline guide](../getting-started.md)
