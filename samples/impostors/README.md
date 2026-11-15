# Complex model and impostor

**Adaptive octahedral impostor: one quad, three views, six to twelve texture reads.**
The layered tracer remains available with `--profile layered` for visual comparison.

The sample compares three downloaded CC0 Poly Haven models with their
**two-triangle octahedral impostors**:

| Model | Source triangles | What it tests |
| --- | --- | --- |
| [Namaqualand Boulder 06](https://polyhaven.com/a/namaqualand_boulder_06) |109,114 | Irregular scan and textured surface |
| [Wooden Stool 01](https://polyhaven.com/a/wooden_stool_01) |10,946 | Thin legs, crossbars and open gaps |
| [Ceramic Vase 02](https://polyhaven.com/a/ceramic_vase_02) |11,128 | Curved body, rim and cavity |

Use **Model** to select a pair. The full model is on the left, its impostor on
the right. Both use the same prepared source, scale, camera and live lighting.
Source geometry is preserved. The model uses prepared 256px color/normal textures;
the impostor captures those materials into its own BC1 atlases.
```sh
mise exec -- go run ./cmd/engine run web ./samples/impostors
```

Drag with the mouse or one finger to orbit the pair. Wheel or pinch changes the
viewing distance. Press **L** or tap **Light** to cycle three sun directions;
the impostor's stored surface normals respond to the same light as the model.
Press **T** or tap **Rotate** to cycle paused, yaw, pitch and roll motion. Both
representations use the same continuous root rotation about the baked model
center. Changing the axis retains the previous orientation, allowing combined
rotations. Press **R** or tap **Reset view** to restore the camera and pause at
the initial object pose. Buttons display the current light and rotation state;
their labels update when those settings change.
Click the canvas before using browser keyboard shortcuts. Zooming out further
allows inspection at the small projected sizes appropriate for a far LOD.

The debug panel starts collapsed so it does not cover the comparison. The
**Debug** button or F3 reopens its draw/instance/resource counters. The same
retained controls are available in the native window; its title also lists the
L/R/T keyboard controls.

Orbit has the engine's usual near-pole limit, with no capture-elevation clamp.
You can inspect the models from above and below or tilt them using pitch/roll.
Two-finger panning stays centered on the pair.
The field of view accounts for a portrait viewport; zooming closer can crop
the pair. Its landscape distance starts at 20 units;
use **Inspect close** under **Light, rotation and view** to examine capture resolution
and silhouette differences.
Since the two objects occupy fixed world positions, orbiting around
their sides can put one behind the other. Use Reset for the initial comparison.

## Source and build recipe

The three source GLBs are retained locally, so normal builds need no network access.
They retain the original mesh/index/UV bytes and use 512px albedo/normal PNGs.
The original 2K texture payloads and unused roughness/ARM images are removed;
there is no high-resolution fallback profile. A node transform uniformly scales
and grounds each model. Both representations use scalar roughness 1 rather than
the source roughness map; this is not full source-PBR reproduction. See the
[copyright annex](#copyright-annex) for authors, sources and adaptations.
There is no sample downloader or source converter.

The recipe builds `assets/boulder.glb`, `assets/stool.glb` and `assets/vase.glb`.
The procedural tree remains only a regression fixture. The retained **Model**
dropdown switches the pair in the running scene on browser and native. Camera,
lighting, rotation and quality settings survive the change. The previous pair is
removed and unloaded after its replacement is ready; only the selected pair
remains instantiated. Model changes perform prepared-asset loading and uploads
on the App thread and can pause presentation briefly; this is not streaming.
The sample embeds all three prepared pairs. The temporary recipe uses about
2 MiB for full-model material textures and 13.61 MiB of BC1 block payload for the
three impostor atlas pairs, plus original geometry and metadata. The published
browser sample uses TinyGo and must fit within 20,000,000 bytes including every exported file.
The temporary 256px TinyGo development preview totals **25,472,564 bytes
(25.47 MB)** across all four files. Its six prepared model/impostor containers
alone occupy 20,226,921 bytes. Development preview works, but the complete size
audit fails the unchanged 20,000,000-byte publishing cap; this setting is not
qualified for gallery publication. The earlier 128px export figure does not
qualify this setting. See [validation](../../spec/validation.md) for measured
build provenance and limits.
This is a comparison bundle containing both representations.

`asset.config.go` declares one `impostors` pipeline with those three GLBs as
its inputs. `modelbuild.GLBWithOptions` imports and publishes the original prepared models;
`impostorbuild.Cards` processes those records and publishes the prepared cards.
Both plugins retain their combined processing/publication API, so this pipeline
needs no separate Output plugin. The profile settings and generated paths remain
shared by the original model and its impostor. It composes:

```text
Tracked GLB → modelbuild.GLBWithOptions → prepared model + impostorbuild.Cards
                                                ↓
                              prepared view atlases + static quad
```

The model remains available in `generated/models`; the impostor catalog is in
`generated/impostors`. The card baker consumes the prepared model produced by the
GLB plugin, so both representations come from the same input and compiler result.
The entire `generated/` directory is disposable and ignored. Prepare it directly:

```sh
mise exec -- go run ./cmd/engine assets build --project ./samples/impostors
# or
mise exec -- go run ./cmd/engine assets generate --project ./samples/impostors
```

Retained GLBs use 512px material maps. The recipe caps imported model maps
at 256px before mip generation, so future replacements follow the same demo
budget. Both representations consume the same prepared model. Source decoding,
resizing and role-aware mips are build work; there is no runtime conversion.

The `dev` and `release` profiles temporarily select `Mode: impostorbuild.Octahedral`,
`Resolution:256`, `FrameGrid:7`, `Padding:8` and `MipLevels:3`. Seven frames per
axis align the full-sphere grid with poles and octahedron fold lines. The public
zero-value `Cards()` layout remains 8×8/80px. This resolution increase keeps the
current 49-view capture method. The proposed six-view method is deferred;
capture-method efficiency will be revisited separately.

The runtime always blends three neighboring views. It uses 0/1/2 depth-based UV
corrections as projected size increases, costing 6/9/12 texture sampling
instructions. Normal/depth and color are sampled at the same final coordinate,
with live lighting and depth intersections. All textures/mips are prepared
offline; camera motion changes neither atlas packing nor residency.

The temporary default 256px views have three mips: 256→128→64px. The 49 captures
form two BC1 atlases with 1904×1904, 952×952 and 476×476 levels. Each pair uses
4,758,096 block payload bytes (4.54 MiB); all three use 14,274,288 bytes (13.61 MiB).
Metadata and executable/host files are additional; these are not complete export
sizes. CPU copies and driver overhead are separate residency costs.
Color carries sRGB RGB and one-bit
silhouette coverage. The linear atlas stores oct normals in R/B and depth in G.
The build creates and compresses every mip; there is no runtime compression or
texture fallback. A GPU with BC support is required. `detailed` is an extended-mip
reference at the same 256px capture resolution, with padding 16 and five mips;
source material maps stay at 256px in every profile.

The **Reprojection controls** switch opens the advanced settings; they start hidden
so the model comparison stays clear. Expanding them is retained across supported
model changes. Capture modes without adaptive quality hide the switch and settings.
The advanced controls support:

- **Auto:** 0 corrections through 16px; blend to 1 over 16–32px; blend to 2 over 128–256px.
- **Reprojection — 2 corrections / 1 correction / 0 corrections:** force the
  corresponding 12/9/6-read reference.
- **Far / Near:** pixel thresholds where each correction starts fading in.
- **Mip bias:** positive samples coarser mips; negative samples finer ones.

Threshold inputs are enabled in Auto; invalid input preserves the last applied
settings. Quality changes update the existing material uniform without rebuilding
assets. They cannot restore capture definition, one-bit coverage or detail lost
to BC1 compression. All instances sharing the loaded asset share the settings. The public
API is `card.SetQuality(impostor.Quality{...})`; see [quality controls](../../pkg/impostor/README.md#quality-controls).

```sh
# Extended-mip reference: the same 256px views, padding 16 and five mips.
mise exec -- go run ./cmd/engine run web ./samples/impostors --profile detailed

# Layered numeric reference:128px axis grids, eight intervals.
mise exec -- go run ./cmd/engine run web ./samples/impostors --profile layered

# Other capture layouts, using their respective runtime paths.
mise exec -- go run ./cmd/engine run web ./samples/impostors --profile octahedral
mise exec -- go run ./cmd/engine run web ./samples/impostors --profile angular
```

The layered reference applies only to the boulder; the stool and vase use
64px octahedral views in that profile. It uses closed-solid occupancy, numeric depth intervals and a
conservative hierarchy. Its 128px atlas pair stores 11,210,752 bytes (10.69 MiB),
with one numeric level. It reconstructs geometry more accurately but can traverse
up to 256 hierarchy nodes per pixel, plus interval and attribute searches. It is
retained as a quality reference, rather than the sample default.

These are offline build settings. Camera and T-key motion update ordinary
transforms and shader projection; they do not rebuild geometry, scan model parts
or repack atlases. The sample caches its transform pointers and baked center
once when each pair loads. Paused objects retain their existing transforms.

## Representation limits

Historical fixed-correction throughput and detailed-profile measurements live in
[benchmark evidence](../../spec/impostor-benchmarks.md). Current adaptive
qualification is recorded in [validation](../../spec/validation.md).

This remains an approximate far LOD. The bounded parallax corrections cannot recover
hidden surfaces; close silhouettes and disocclusions can distort and material
detail softens during view blends. The initial enlarged view exposes these
limits; zoom out to judge the intended LOD range. Camera/object rotation stays
continuous, but this path does not reproduce the tracer's geometric fidelity.
The stool exposes torn crossbars and disocclusion errors when enlarged; the
vase retains its broad silhouette better, with softer detail and rim errors.
There is no automatic LOD selection. Each downloaded source has one material
primitive and its proxy one card; the common ground and debug panel contribute additional work.

The earlier procedural tree is an internal regression fixture. Its authoring
source, golden GLB and regeneration instructions are owned by
[internal/impostorfixture](../../internal/impostorfixture/); the gallery recipe
uses only the three downloaded comparison models.

## Code organization

`main.go` owns the common `main`/`run` application lifecycle. `scene.go` creates
the selected camera, lights and frame updates; `presentation.go` loads the chosen
prepared pair and owns its overlays, instances and resources. Failed setup rolls
back acquired resources, and cleanup errors join the returned application error.
Quality application failures propagate through `OnUpdateError`; invalid user
quality input still preserves the last applied settings. `interface.go` creates
one nonmodal, scrollable `pkg/ui` overlay on both platforms, using the prepared
default font and sharing the View with diagnostics. There are no HTML controls
or page reloads. The shared sample helper adjusts the perspective field of view
when the viewport changes; there are no platform-specific source files. `asset.config.go` owns asset generation and playable preparation, preserving every capture profile.

## Copyright annex

These Poly Haven models use [CC0 1.0](https://creativecommons.org/publicdomain/zero/1.0/).

| Asset | Source | Author |
| --- | --- | --- |
| `assets/boulder.glb` | [Namaqualand Boulder 06](https://polyhaven.com/a/namaqualand_boulder_06) | Greg Zaal (photography), Jenelle van Heerden (modeling) |
| `assets/stool.glb` | [Wooden Stool 01](https://polyhaven.com/a/wooden_stool_01) | Kuutti Siitonen |
| `assets/vase.glb` | [Ceramic Vase 02](https://polyhaven.com/a/ceramic_vase_02) | James Ray Cock |

Karty preserves the source mesh/index/UV bytes and node placement, with a uniform
node transform to center/ground each model at radius 3. The original 2K albedo
and OpenGL normal maps were reduced offline to 512px PNGs using linear-light
color filtering and renormalized normal filtering. Unused roughness/ARM images
were removed; materials use scalar roughness 1 and metallic factor zero. The
recipe further caps material maps at 256px for both displayed representations.
