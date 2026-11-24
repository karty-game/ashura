# Sponza

The sample publishes each chunk's geometry, UVs and RNM lightmaps as one stored
ZIP for the selected desktop or mobile profile. Material textures remain shared
external resources, so neighboring chunks can reuse them. Packing happens in
the asset recipe; loading verifies and views the prepared payloads without
decompressing or converting them.

Sponza demonstrates the standard offline level asset pipeline: load a GLB,
prepare materials, separate cutouts, partition opaque geometry, unwrap lightmap
UVs, trace RNM lighting, reconstruct it and publish portable GPU textures.
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
preparation use embedded offline libraries. See the
[asset pipeline tool requirements](../../docs/asset-pipeline/tools.md).

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

## Views

Expand **Views** for the retained camera buttons.
The initial panel shows only the scene title and loading status; diagnostics
start collapsed. Keyboard shortcuts work with the panel collapsed.

Use **1 / Courtyard**, **2 / Gallery**, **3 / Free flight** or **B / Ceiling**.
**R** resets the selected view. Drag to look; wheel/pinch zooms orbit views and
two-finger dragging pans. In free flight, WASD moves and Space/Shift rises or
descends. The retained UI buttons provide the same views on browser and native. **F3** toggles diagnostics.
Navigation remains active while prepared resources load; there is no collision
or gravity.

The scene always uses RNM with SSBump relief on browser and native.
`level.Open` selects the prepared BC or mobile texture family for the device
and owns its loading and cleanup. Camera changes retain that presentation;
there is no lighting selector or RGB sample profile.

The shared level reader overlaps up to eight missing chunk-file downloads and
their checksum verification, within a 32 MiB active payload window. The browser
can multiplex these requests over HTTP/2 when the server supports it. Asset
admission and GPU uploads retain their bounded App-loop steps, so camera and UI
updates continue while resources arrive. The independent cutout model loads
alongside the chunk; no whole-world preload or runtime texture conversion is added.

The camera buttons, chunk status, help and attribution use
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
material binding, chunking, UVs and lighting follow. `PublishVariants` handles
final GPU encoding and publication for the two RNM device profiles. No standalone
intermediate KTX2 copies are published. Authored settings and material-to-relief
bindings live below the main sequence in the same file; preparation algorithms belong to the shared
modules. Its standard `//go:build ignore` marker isolates the host recipe from
the game executable; sample runtime and tests use no platform build directives.
`sponza.scene.yaml` declares the model and static courtyard lamp.

The recipe runs explicit, independent geometry stages: `meshbuild.Cleanup()`
before scene import/xatlas, then `meshbuild.Optimize()` after `LightmapUVs` and
before `ProductionBake`. The latter orders final seam vertices and triangles for
GPU cache/fetch locality, remaps UV1/receiver references and updates model digests.
Material/UV seams, receiver footprints, texel density and bake settings are preserved;
no simplification, quantization or load-time mesh decoding is introduced. See
the [mesh build module](../../pkg/mesh/build/README.md).
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

`PublishVariants` publishes only `RNMBC` and `RNMMobile` from that bake. Desktop
uses BC7 RNM lightmaps/relief and BC1 albedos. Mobile uses ETC2 RGB lightmaps/albedos
and exact linear RGBA8 relief; those level payloads need no optional BC/ASTC
support. ETC1S is an offline albedo encoding intermediate. The game uploads native GPU payloads and
requires no codec/transcoder download, source decoding or runtime repacking.

The desktop profile stores SSBump relief in BC7, preserving the authored
dimensions and mip chain. The remaining RGBA8 relief belongs to the portable
mobile profile. BC7 requires the optional BC texture-compression feature, and
the tested ETC2 encoders introduced excessive directional response error.

The ignored `generated/` directory holds typed descriptors, compiled scene
metadata and public payloads. The BC and mobile material textures and chunk
packs live under `public/texture-profiles/bc/` and
`public/texture-profiles/mobile/`. Manifests and independent dynamic models
remain under `public/levels/`. The 45,000,000-byte budget applies to each
profile and includes WASM, its profile files and all common files. The
gallery total includes both profiles. All measurements use logical file lengths.
See [production lightmap baking](../../pkg/lightmap/bake/README.md) and
[prepared level publication](../../pkg/level/build/README.md).

## Copyright annex

The [Sponza asset credits](assets/CREDITS.md) document licenses, sources and
adaptations for the model, textures and relief maps. The web export includes
the same credits.
