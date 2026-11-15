# Sponza

Sponza demonstrates the standard offline level asset pipeline: load a GLB,
prepare materials, separate cutouts, partition opaque geometry, unwrap lightmap
UVs, trace RGB/RNM lighting, reconstruct it and publish portable GPU textures.
The game opens the generated descriptors through `level.Open`, which owns
prepared chunk and independent-model loading, sharing and cleanup.

The selected preset is **32 samples, 3 diffuse bounces and strong denoising**
(`NoiseScale: 128`). The surface filter reaches two texels (0.25 world units),
with a 0.025-unit plane tolerance and unchanged normal/tangent guards. It keeps
8 lightmap texels per world unit, 256px SSBump relief and
8× anisotropy. Receiver UVs use 512px pages, padding 4 and preserved geometric
normals; publication consolidates their charts into atlases up to 1024px. The
sample uses one 32-unit chunk. Small lighting variations soften; some broad
blotching remains in dark areas. These are preview assets pending final directional
material-normal qualification, not a noise-free convergence claim.
Compensated Russian roulette starts after the first reflection: low-throughput
paths terminate probabilistically and surviving paths carry the reciprocal
survival weight (`RouletteDepth: 1`). The three-bounce expectation is preserved,
with higher variance in later indirect contributions at this fixed sample count.

## Run

From the repository root, with required mise 2026.11.5 or newer and `mise install` completed:

```sh
mise exec -- go run ./cmd/engine run web ./samples/sponza
```

The same sample appears in `mise run samples`. The CLI runs `asset.config.go`
before compiling and serves an immutable distribution on a free loopback port.
`mise exec -- go run ./cmd/engine assets generate ./samples/sponza` prepares assets explicitly. The
asset modules use embedded xatlas and Basis WASM libraries through Go for UVs
and texture encoding. No sample tool directory, source-conversion program,
JavaScript codec runner or manually selected executable is part of the project.
GPU baking runs in Go through the shared WebGPU binding; mise installs its
checksum-pinned upstream wgpu-native library when needed. UV and texture
preparation no longer depend on separate Karty native tool releases.
[Dependency availability and host qualification limits](../../spec/asset-tools.md)
remain distinct from actual hardware execution.

On Linux, install the system Vulkan loader and your GPU's Vulkan driver before
baking. For AMD/Intel on Debian or Ubuntu:

```sh
sudo apt-get update
sudo apt-get install libvulkan1 mesa-vulkan-drivers
```

Mise installs the wgpu-native library, while these system packages provide its
Vulkan runtime and hardware driver. A working OpenGL browser adapter alone does
not supply the native baker's Vulkan dependency.

Default builds require a hardware compute adapter. The explicit
`KARTY_LIGHTMAP_SOFTWARE_FOR_CI=1` module option admits native software Vulkan
for functional CI checks only:

```sh
KARTY_LIGHTMAP_SOFTWARE_FOR_CI=1 mise exec -- go run ./cmd/engine assets generate ./samples/sponza
```

This is a declared build mode, not an automatic fallback. Local visual review
still requires hardware. Native viewing, after asset preparation, runs from the
repository root with
`env CGO_ENABLED=0 GOFLAGS=-tags=nofakecgo mise exec -- go run ./samples/sponza`;
its resource base is `samples/sponza/generated/public`. The tag selects the shared
GoFFI/purego callback runtime without enabling cgo. The native device enables
supported compression features and uploads the selected prepared BC or ETC2
profile directly; there is no runtime image decoding or recompression.
The shared `resource.ProjectBase` API selects that native path or the browser
export's relative public root. The sample needs no platform-selection files.

## Views and lighting

Expand **Views and lighting** for the retained camera buttons and lighting selector.
The initial panel shows only the scene title and loading status; diagnostics
start collapsed. Keyboard shortcuts work with the panel collapsed.

Use **1 / Courtyard**, **2 / Gallery**, **3 / Free flight** or **B / Ceiling**.
**R** resets the selected view. Drag to look; wheel/pinch zooms orbit views and
two-finger dragging pans. In free flight, WASD moves and Space/Shift rises or
descends. The retained UI buttons provide the same views on browser and native. **F3** toggles diagnostics.
Navigation remains active while prepared resources load; there is no collision
or gravity.

RNM with SSBump relief is the default. The retained **Lighting** dropdown selects
RNM or RGB on both platforms. A change cancels and joins the previous asset
reads, releases its scene resources and loads the selected prepared profile
through `level.Open`. The World and camera controllers remain attached.
Only the selected profile/device family is fetched; selecting the current mode
keeps its resources. Browser query parameters no longer select lighting.

The camera buttons, lighting selection, chunk status, help and attribution use
`pkg/ui` and its prepared default font. The nonmodal, scrollable panel shares the
App's View with diagnostics and adapts to the viewport. Status changes only on
loading events. There are no sample DOM controls or `syscall/js` imports.

## Source and pipeline

`assets/` contains one model, `sponza.glb`, and the ten grayscale relief textures
in `relief/`. The GLB contains architecture, fabrics, chains and vase foliage:
125,241 source triangles, eighteen material groups and their embedded images.
Geometry, UVs and source image bytes were preserved when consolidating the
previous opaque and cutout inputs. See the [copyright annex](#copyright-annex)
for asset credits, sources and adaptations.

`asset.config.go` declares one named `world` pipeline with `Import`, `Process`
and `Output` plugin lists. Its inputs include the model, relief images and scene
because material and scene processing depends on those model records. They are
imported first; resize, relief, mips,
material binding, chunking, UVs and lighting follow. `PublishPortable` handles
final GPU encoding and publication. No standalone intermediate KTX2 copies are
published. Authored settings and material-to-relief bindings live below the
main sequence in the same file; preparation algorithms belong to the shared
modules. Its standard `//go:build ignore` marker isolates the host recipe from
the game executable; sample runtime and tests use no platform build directives.
`sponza.scene.yaml` declares the model and static courtyard lamp.
`Scenes(SeparateMasked: true)` retains opaque surfaces in the bake and
publishes masked surfaces as independent, world-space prepared models. Cutout
opacity, material cutoff, original UVs and indices survive. They remain visible
at runtime and do not cast baked or dynamic shadows in this sample yet.

The in-process Go compute baker traces combined direct, sky/ground and bounced light.
Fixed sampling submits all pages in one logical-space action through the standard
`BakeBatches` API, retaining independent page identities and outputs. Receiver
preparation overlaps independent scene preparation within the lightmap module.
One visibility- and variance-guided GPU pass smooths the four linear response
statistics before RNM fitting. Its .25-unit radius, plane and frame guards
prevent unrelated surfaces from contributing. Stronger range weights make
low-sample noise less likely to be mistaken for a lighting edge. Reconstruction
never blurs encoded atlas pixels across charts. Halo scheduling uses actual
device, packet and worker-memory limits (`MaximumHalo: 0`); support is never
truncated to fit a receiver quota. The calibrated fit uses exact directional
moments and no lookup table.

`PublishPortable` prepares both RGB and RNM profiles from that same bake. Desktop
uses BC7 lightmaps/relief and BC1 albedos. Mobile uses ETC2 RGB lightmaps/albedos
and exact linear RGBA8 relief; those level payloads need no optional BC/ASTC
support. ETC1S is an offline albedo encoding intermediate. The game uploads native GPU payloads and
requires no codec/transcoder download, source decoding or runtime repacking.

Both desktop profiles already store SSBump relief in BC7, preserving the authored
dimensions and mip chain. The remaining RGBA8 relief belongs to the portable
mobile profiles: BC7 requires the optional BC texture-compression feature, and
the tested ETC2 encoders introduced excessive directional response error. See
the [coefficient comparison](../../spec/baked-level-v7.md#bounded-quality-evidence-and-limits).

The output is ignored `generated/`: typed `levels/` descriptors, compiled scene
metadata and `public/levels/` payloads. The normal CLI stages only that active
publication. Historical comparison outputs elsewhere under `generated/` are
not part of the exported sample. The complete exported sample has a
35,000,000-byte budget, including its WASM, host files and all four published
RGB/RNM device profiles. Count logical file lengths across the whole export;
filesystem compression and selecting only one profile do not reduce that total.
See [lightmapper design](../../spec/lightmap.md),
[portable delivery](../../spec/baked-level-v7.md) and
[validation evidence](../../spec/validation.md).

## Copyright annex

**Crytek Sponza**, © 2010 Frank Meinl / Crytek, is inspired by Marko Dabrovic's
original Sponza (2002). Asset notices below are separate from the repository's
[MIT source-code license](../../LICENSE). No endorsement by the original authors
is implied.

The model and diffuse textures come from Babylon.js's
[Sponza Dynamic Shadows demo](https://www.babylonjs.com/Demos/SponzaDynamicShadows/)
and its [source scene](https://www.babylonjs.com/Scenes/SponzaDynamicShadows/SponzaDynamicShadows.babylon).
Babylon maintainers confirmed reuse permission in their
[October 2022 discussion](https://forum.babylonjs.com/t/whats-the-license-on-the-sponzadynamicshadows-scene/34399).
The archive license below applies to the relief sources; it is not asserted as
the license of Babylon's separate adaptation.

Karty's offline adaptation flattens translations, converts Z and texture V
conventions, indexes position/normal/UV tuples and merges compatible materials.
Opaque diffuse JPEG bytes are preserved; chain and foliage diffuse/mask JPEGs
are combined into RGBA PNGs with alpha cutoff 0.5. Collision proxies, sky,
embers, the moving lamp marker and existing AO/lighting textures are excluded.
The consolidated GLB preserves the prior opaque/cutout geometry and image buffers.

Nine unmodified height maps come from Morgan McGuire's
[Computer Graphics Archive](https://casual-effects.com/data/),
[Crytek Sponza download](https://casual-effects.com/g3d/data10/common/model/crytek_sponza/sponza.zip),
whose [model entry](https://casual-effects.com/g3d/data10/common/model/crytek_sponza/info.js)
declares [Creative Commons Attribution 3.0 Unported](https://creativecommons.org/licenses/by/3.0/legalcode).
McGuire's July 14, 2011 modifications corrected the Crytek OBJ export, derived
height maps from normals using `normal2bump.cpp`, combined diffuse/mask textures,
cleaned masks, supplied `gi_flag.tga` and separated the untextured atrium banner.
The retained height-map sources are `lion_bump.png`, `background_bump.png`,
`sponza_arch_bump.png`, `spnza_bricks_a_bump.png`, `sponza_column_a_bump.png`,
`sponza_column_b_bump.png`, `sponza_column_c_bump.png`, `vase_bump.png` and
`vase_round_bump.png`, renamed to `*-height.png` roles here.
`floor-height.png` is Karty-authored. SSBump generation is an offline Karty
adaptation; the generated coefficients are not Valve-exported assets.
