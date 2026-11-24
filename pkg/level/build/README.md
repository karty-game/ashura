# Level asset plugins

Use explicit `meshbuild.Cleanup()` after prepared model import and before scene
import. Place `meshbuild.Optimize()` after `LightmapUVs` and before
`ProductionBake` to order final seam geometry for vertex cache/fetch locality.
Receiver references, UV1 and model hashes remain bound to the final mesh. See the
[offline mesh module](../../mesh/build/README.md) for scope and portable tooling.

For named pipelines, put `Scenes(SceneOptions{...})` after model/image imports in
`Pipeline.Import`. Use `Materials(MaterialOptions{...})` in `Pipeline.Process` after
resize, SSBump generation and mip filtering, followed by chunking, UVs and baking.
Finally put `Publish` or `PublishVariants` in `Pipeline.Output`. Materials accepts processed
image records directly; no intermediate `KTX2()` file publication is needed.
See [Sponza's recipe](../../../samples/sponza/asset.config.go) and the
[asset pipeline guide](../../../docs/asset-pipeline/README.md).

`LightmapUVs(UVOptions{Production: true, ...})` and `ProductionBake` select the
new offline compute path. It supports physical RGB and calibrated RNM from one
trace, hierarchy-sampled emitters, footprint antialiasing, bounce lighting,
independent pilot allocation and cross-page linear reconstruction. Use
`PublishOptions.Representation` with `level.RGBDiffuse` or `level.RNMFit` to
publish one or three maps. See the [baker API](../../lightmap/bake/README.md)
and [level runtime format](../README.md). `ProductionOptions.Worker.MemoryBytes`
is an explicit offline worker ceiling. The baker detects the native backend
identity for caching. Final RNM requires `Verification`, all
receivers and the actual material normal domain. Explicit previews retain
verification diagnostics and are marked in the manifest.

`samples/sponza/asset.config.go` is the maintained preview recipe and uses
`PublishVariants` to emit only RNM BC and RNM mobile native profiles.

Set `PublishOptions.BundleChunks: true` to publish each chunk's geometry,
secondary UVs and lightmaps as one uncompressed pack. Shared material textures
and independent models remain separate files. The build records byte offsets
in the v8 manifest. Loading uses verified byte views without an archive parser
or decompressor. The option defaults to
false and applies independently to each publication profile.

For integrated preparation, follow `StaticChunks` with
`ProductionBake(ProductionOptions{UVs: &UVOptions{TexelsPerUnit: 16,
MaxPageSize: 512, Padding: 4}, ...})`. The baker prepares production receiver
UVs while independently building acceleration and light distributions from the
same complete chunk visibility geometry. Omit the separate `LightmapUVs` step
in this form. Leaving `UVs` nil preserves the separately staged form above.

Use the native production recipe below for maintained level builds. The inert
`RNMBake` function and `BakeOptions` type have been removed from this unreleased
API. Replace older recipes with `LightmapUVs(UVOptions{Production: true, ...})`
and `ProductionBake(ProductionOptions{...})`. Migrate lighting calibration and
verification policy explicitly; existing `.klightmap` sidecars are not
automatically converted. The old `engine assets lightmap` command still returns
migration guidance before IO. Run the production recipe with `engine assets build`.

For native web delivery, use v7 normalized LDR variants.
Prepare BC and ETC2 profiles with `PublishVariants`, select the matching
descriptor after device initialization, and upload prepared textures without
embedding a runtime decoder.

`PublishOptions.AlbedoETC1S` optionally enables the v3 web delivery profile.
Select an explicit quality; an empty executable uses the embedded Basis library.
Explicit native executable paths and non-system components are qualification
overrides. Distinct albedos encode after chart consolidation; input mips and all
RNM/SSBump bytes remain intact. The loader transcodes to GPU compressed blocks
in a dedicated web worker. This option currently requires the browser loader;
existing native/standalone loading continues to support uncompressed profiles.

`PublishOptions.NativeTextures` selects v7 normalized LDR delivery.
Use `NativeTextureOptions{Family: "bc", LightmapFormat: "bc7", Transfer: "srgb", ...}`
for the measured desktop choice. Optional BC1 trades precision for 4 bpp; the
portable mobile target is ETC2 RGB. Lightmaps keep one common RGB/RNM tuple scale
and use existing hardware sRGB sampling. The builder shares opaque UASTC/native
encoding and requires no HDR encoder or runtime decoding component. NativeAlbedo
retains the explicit offline AlbedoETC1S-to-native preparation path.
`ProfileMaterials` places native albedo and relief under the selected device
profile's directory. It requires `NativeAlbedo`. Do not set
NativeTextures and NativeHDR together. Mobile SSBump keeps exact linear RGBA8
channels and authored mips in a canonical GPU-ready KTX container because the
tested ETC2 coefficient encoders lose too much directional precision. Desktop
SSBump remains BC7. No ASTC capability is assumed and no decoder is added.
Select `Encoder.SSBumpQuality` explicitly when trading build time against
desktop relief precision. Sponza uses effort 0; raw publication defaults to 4.
Device variants share identical page packaging, atlas and codec producers;
their geometry, UVs, materials and delivery descriptors retain separate bounds.
Native LDR variants prepare pages, atlases and normalized maps together, then
run independent material and lightmap codecs in one dependency graph. Native
albedo transcoding consumes its declared ETC1S producer directly. A combined
profile index binds the finished material and lightmap files before descriptors
are published; an encoder failure still rejects the complete build output.
Physical mobile and smooth-loading qualification remain open.

Legacy `PublishOptions.NativeHDR` enables the v4 BC delivery profile with
offline shared-exposure BC6H RNM and BC7 SSBump encoding after consolidation.
It uses the embedded offline codecs by default and requires a BC-capable browser. The GPU receives
native blocks; this option adds no runtime codec. `ProductionOptions.Sampling`
selects offline tracing quality; `Reconstruction` controls geometry-guided
filtering. Set `UVOptions.PreserveGeometricNormals` for production tracing; its
guide artifacts stay in the build cache. `ReconstructionOptions.MaximumHalo: 0`
uses actual packet, device and worker-memory bounds without an additional
receiver ceiling; a positive value remains an explicit scheduling cap. Complete
neighbor support is preserved in both modes.

These host-only plugins extend the ordinary asset recipe. Put prepared model and
material producers first. The same scene schema/compiler resolves levels and
prefabs; a level adapter does not implement a second YAML interpretation.

```go
Output: "generated",
Pipelines: []assetbuild.Pipeline{{
    Name: "world",
    Input: []string{"models/**", "levels/**"},
    Import: []assetbuild.Plugin{
        modelbuild.PreparedGLB(),
        levelbuild.Scenes(levelbuild.SceneOptions{
            Input: []string{"levels/main.scene.yaml"},
        }),
    },
    Process: []assetbuild.Plugin{
        levelbuild.Materials(levelbuild.MaterialOptions{}),
        levelbuild.StaticChunks(levelbuild.ChunkOptions{
            Size: [3]float32{32, 32, 32},
        }),
        levelbuild.LightmapUVs(levelbuild.UVOptions{
            Production: true, TexelsPerUnit: 16, MaxPageSize: 1024,
            Padding: 4, PreserveGeometricNormals: true,
        }),
        levelbuild.ProductionBake(levelbuild.ProductionOptions{
            Worker: bake.WorkerOptions{
                MemoryBytes: memoryBytes,
            },
            Sampling: bake.Options{
                Outputs: bake.OutputRGB | bake.OutputRNM,
                Samples: 256, Bounces: 4, Seed: 29,
            },
            Environment: &bake.Environment{
                Sky: bake.Vec3{.12, .14, .18},
                Ground: bake.Vec3{.025, .02, .015},
            },
            Preview: true, // Final RNM also requires complete directional verification.
        }),
    },
    Output: []assetbuild.Plugin{levelbuild.Publish(levelbuild.PublishOptions{})},
}},
```

`PreparedGLB` exposes cached prepared-model slots without embedding/publishing
all original models. Use `modelbuild.GLBWithOptions` when original
embedded models are also wanted. `SceneOptions.Modules` defaults to logic/render3d;
custom modules preserve their ordinary generated constructors and logical state.

`SceneOptions.Spaces` maps node paths to logical space identities; descendants
inherit the closest explicit assignment. This preserves ownership for overlapping
spaces without inventing portal connections. `SSBump` maps model resource identity
to exact original primitive name to a prepared SSBump resource identity. Place
`Images` before scene import and `Resize`/`SSBump`/`Mipmaps` before `Materials`
for authored height relief. `MaterialOptions.SSBump` owns these explicit bindings. Normal-derived relief is prepared offline where an explicit
override is absent. Automatic derivation uses square power-of-two normal maps,
a mip at most 512 pixels wide and the material's normal strength. Supply an
explicit override for other normal-map profiles. Unknown primitive names in
used models fail the build instead of silently dropping their relief overrides.

## Lighting policy

Point lights remain live by default. Opt a source into the static bake explicitly:

```yaml
- id: lamp
  components:
    spatial.transform3d: {position: [2, 3, 1]}
    render3d.point:
      enabled: /power
      bake: static
      color: [1, 0.8, 0.6]
      intensity: 4
      range: 12
```

A static light bakes its enabled binding's authored initial state. Its generated
live intensity is zero, preventing double counting. Changing its logical boolean
later does not modify the completed lightmaps. Runtime-controlled illumination
should remain dynamic until a rebake/scenario system is connected.

Authored hemisphere intensity contributes white sky and `[.35, .4, .45]` ground
colors. `ProductionOptions.Environment` supplies an explicit replacement. Authored
point lights use the named `LegacyRangeV1` adapter so their existing range and
intensity semantics remain explicit. Additional physical emitters come from
`ProductionOptions.Emitters`, keyed by logical space. Lights and mesh mobility
are independent concepts.

Chunk receivers query the complete logical space's visibility and reflected
surfaces. A distant occluder or reflector can affect a chunk across several cell
boundaries. The production solver samples the prepared emitter hierarchy; it does
not restrict transport to the receiver chunk's box. Historical browser solver
admission and light-list limits do not describe the production worker.

One build-scoped native hardware worker shares its device and production WGSL
pipelines. Fixed-count pages in a logical space use one `BakeBatches` action with
independent page outputs; adaptive and explicit-resume jobs retain per-page
planning. The executor uses the same native Go WebGPU binding as rendering,
without a helper process, browser, window or loopback service. Per-job buffers
release after readback; resident scenes and pipelines are reused. Our baking
logic remains a reusable Go/WGSL package called by the CLI's asset recipes.

`ProductionBake` probes the selected native adapter once per pipeline and uses
its reported device, driver and backend as inputs to the sampling and resolve
cache keys. The probe does not create a compute device or compile shaders. On a
cache miss the worker checks that it selected the same adapter before baking.
`BackendIdentity` overrides detection when the recipe supplies an explicit
identity. Offline actions also fingerprint the actual native library using
`Context.TrackLibrary`; the recipe binary includes our Go implementation and WGSL.
The pipeline installs the checksum-pinned upstream native library through mise
when needed; explicit `WGPU_NATIVE_PATH` and existing initialized libraries remain
available. No worker release or native consumer compilation is needed.
See [asset pipeline tool requirements](../../../docs/asset-pipeline/tools.md).

## UV preparation tools

`LightmapUVs` uses embedded xatlas WASM through Go/wazero for production and
legacy bake inputs. Normal recipes declare UV settings only; no compiler,
executable installation, JavaScript runtime or xatlas release is required. The
build-owned preparer reuses compiled code and each request owns an isolated
instance. Go repairs overlapping/collapsed charts before the existing density,
geometry and receiver admission. See [lightmap UV preparation](../../../docs/asset-pipeline/processors/level-lightmap-uvs.md).

`UVOptions.Compiler` and `Executable` have been removed, along with
`KARTY_XATLAS`. UV preparation always uses the embedded library. Third-party
source is never compiled during preparation, and no encoder helper enters a
game build.

## Cache and publication

A small discovery manifest expands into ordinary cacheable chunk/page actions.
Large payloads use checksummed auxiliary artifacts rather than a giant base64
bundle. Each public payload has its own bounded publication action. Final output
publishes only after every producer succeeds; processing failures preserve the
previous complete level. Corrupt or missing auxiliary cache blobs invalidate
their producers.

An unchanged build reuses its actions. Light-only edits preserve chunk partition
and UV/transport preparation. Geometry or reflectance changes conservatively
invalidate lighting/transport within the logical space; exact ray-dependency
invalidation is not implemented. Density/grid edits rebuild their dependent stages.
These reuse guarantees assume unchanged recipe code and executable identity;
changing literal recipe options recompiles that identity and invalidates all
action candidates. Every action must also declare all output-affecting options,
such as the generated descriptor package, for runtime-configured options to cache
correctly.

Output contains `public/levels/<hash>.json`, content-addressed geometry/UV files,
shared KTX2 materials, three RNM maps per page, a small generated `levels` descriptor
package and typed logical factories in `scenes`. `PublishOptions.Package` changes
the descriptor package; `Development: true` additionally emits prepared bake
inputs referenced by each page's optional `BakeInputs`. These `.klightmap`
sidecars bind to world-space bake geometry rather than localized production
geometry. They are RNM baker inputs, not runtime model lightmaps. Production loaders do not fetch them. The production output does not
include full source models for static geometry.

Production publication packs baked UV islands into final atlases and merges
compatible Source material ranges within each atlas. Bake-job pages are internal
preparation units, not required production draw boundaries. `PublishOptions`
defaults to a maximum final atlas size of 1024; `MaxAtlasSize: 2048` permits
2048px RNM maps using `karty.baked-level@2`. Position, normal, UV0 and lighting
texels remain unchanged; UV1 adjustment and texture packing happen offline.
Different numeric scales/encodings stay independent. Development output retains
the individually rebakeable input pages.

The initial Sponza output had 415 page/material ranges against the original 22,
which was a performance regression. Consolidation removes that coupling; it
does not merge different material textures into an atlas or array. Even optimal
per-chunk batching may repeat a material across selected chunks. Chunk selection
can limit residency, but automatic streaming selection remains application work.

Per-action, artifact and GPU binding bounds do not bound total offline host
memory to one page: the runner retains action outputs and final publication
payloads until the build completes. Measure peak memory for the target level.

## Sponza

The maintained sample uses its ordinary project recipe:

```sh
mise exec -- go run ./cmd/engine assets generate ./samples/sponza
```

The [Sponza sample](../../../samples/sponza/README.md) imports `assets/sponza.glb`
and `sponza.scene.yaml`, then uses one occupied 32-unit cell at origin
`[-16, -2, -8]`. Its separate production UV stage retains 8 texels per unit, 512px pages,
padding 4 and geometric normals. It traces 32 samples and three bounces with
compensated roulette depth 1, then applies strong .25-unit reconstruction with
noise scale 128 and automatic complete-halo scheduling. Only RNM BC and mobile
profiles publish beneath `generated/public/levels/`, with descriptors in
`generated/levels/`. UV and texture modules run embedded WASM libraries; no
sample-specific tool script or executable configuration is required. The manifest remains a
preview until the complete material-normal domain passes directional verification.


Explicit `engine assets build --jobs 2` permits the production lightmapper's
independent CPU surface-support action to overlap GPU sampling. Shared
helper calls remain exclusive, spaces retain independent ownership, and all
support/sample inputs join before resolve. Zero/one/default remains serial.
This build-only concurrency uses additional resident memory. See
[asset build concurrency](../../assetbuild/README.md#explicit-build-concurrency).


## Portable single-source level preparation

`SceneOptions.SeparateMasked` separates masked parts of static mixed-material
models from opaque chunking/baking. It preserves their material/coverage payloads
in independent world-space prepared models with identity placements. The
manifest retains an independent object per source placement; model IDs include
content identity so identical payloads can share resources. Conflicting derived
object IDs fail, and masked-only relief is not processed for the opaque bake.
Without this option, the prior explicit opaque-only admission remains active.

Use `Publish` for one explicit profile or `PublishVariants` for several profiles
from one bake. Their offline texture paths use embedded Basis libraries, with
no KTX executable or package install. `ProductionBake` uses the in-process Go
executor and native wgpu-native library. See
[asset pipeline tool requirements](../../../docs/asset-pipeline/tools.md).

The [Sponza project recipe](../../../samples/sponza/asset.config.go) demonstrates
the complete stage declaration. Application code uses `level.Open` to own the
prepared level and independent models; it never imports build modules.
