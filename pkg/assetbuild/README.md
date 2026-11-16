# Project asset recipes

Start with the [asset pipeline guide](../../docs/asset-pipeline/README.md) for
plugin-by-plugin descriptions, settings, defaults and recipe examples.

`assetbuild` is a host-only Go package. Projects compose build plugins in
`asset.config.go`; the CLI runs that recipe for development generation and before
compiling the game. Processing and source codecs stay outside native/browser
runtime dependencies.

```sh
mise exec -- go run ./cmd/engine assets generate --project ./samples/standard-materials
mise exec -- go run ./cmd/engine assets build --project ./samples/standard-materials --profile release
mise exec -- go run ./cmd/engine assets build --project ./samples/standard-materials --no-cache
```

The [material comparison recipe](../../samples/standard-materials/asset.config.go)
prepares shared Standard and Valve/Source maps. The
[UI recipe](../../samples/ui/asset.config.go) demonstrates atlas packing.
For an existing game's images, a recipe can be as small as:

```go
//go:build ignore

package main

import "github.com/karty-game/karty-webgpu/pkg/assetbuild"

func main() {
    assetbuild.Main(func(env assetbuild.Environment) assetbuild.Config {
        return assetbuild.Config{
            Output: "generated/assets",
            Pipelines: []assetbuild.Pipeline{{
                Name: "textures", Input: []string{"assets/**"},
                Import: []assetbuild.Plugin{assetbuild.Images()},
                Process: []assetbuild.Plugin{
                    assetbuild.Resize(assetbuild.ResizeOptions{MaxDimension: 1024}),
                    assetbuild.Mipmaps(),
                },
                Output: []assetbuild.Plugin{
                    assetbuild.KTX2(),
                    assetbuild.GoCatalog(),
                },
            }},
        }
    })
}
```

Each `Pipeline` has a unique Go identifier `Name`, its own `Input` patterns and
ordered `Import`, `Process` and `Output` plugin lists. `Config.Output` is the
shared generated root directory; `Pipeline.Output` selects output plugins.
Inputs, logical resources and action/expansion IDs are scoped to the pipeline.
Dependent stages, such as a scene and its models/materials, belong in one pipeline;
there is no implicit resource sharing or dependency API between pipelines. Two
pipelines may import the same source for different processing variants, but their
published filenames must remain distinct. Names do not add output subdirectories.
Set optional `Pipeline.Directory` to a portable relative subdirectory beneath
`Config.Output` when exporters need separate destinations. The builder prefixes
output bundles at final publication; plugin paths and generated package names
remain relative within that directory. Empty Directory preserves current paths.
Traversal, absolute paths and symlink escapes are rejected.

For example, UI and sprite exporters both emit `assets.go`. Give them separate
publication directories when composing them in one recipe:

```go
Output: "generated",
Pipelines: []assetbuild.Pipeline{
    {
        Name: "ui", Directory: "ui", Input: []string{"assets/ui/**"},
        Import: []assetbuild.Plugin{assetbuild.Images()},
        Output: []assetbuild.Plugin{uibuild.Images()},
    },
    {
        Name: "sprites", Directory: "sprites", Input: []string{"assets/sprites/**"},
        Import: []assetbuild.Plugin{assetbuild.Images()},
        Output: []assetbuild.Plugin{spritebuild.Images()},
    },
},
```

Here `uibuild` and `spritebuild` name the UI and draw2d build packages. The
catalogs live at `generated/ui/assets.go` and `generated/sprites/assets.go`; an
application can import those directories with separate aliases.


All pipelines share one cache and atomic final publication. A failure or conflicting
published filename after Directory prefixing prevents publication of the complete build. Plugins that already
publish during import or processing keep that behavior; an empty Output list is
allowed. The lists describe recipe order rather than enforce a plugin type system.

`Config.Input`, `Config.Plugins` and top-level `Import`, `Process`, `Export` are
Deprecated compatibility adapters. New recipes use `Pipelines`. At admission,
older recipes normalize to one pipeline with their historical unprefixed action
IDs and cache identities; they use the same planner and scheduler as named
pipelines.
`Pipelines` cannot be combined with top-level `Input` or either legacy plugin form.
A nonempty flat `Plugins` list also cannot be mixed with nonempty top-level sections.
These groups do not change `Environment.Phase` described below. See the
[V5 pipeline contract](../../spec/asset-pipeline-v5.md) for scope and execution rules.

Keep this recipe and source images in Git. Ignore `generated/assets/` and
`generated/cache/`. Run `engine assets generate --project .` before developing to
make the generated Go declarations and prepared assets available. After editing,
`engine assets build --project .` refreshes playable assets. Both commands run
the same cached recipe; unchanged work is reused. Browser build and preview
commands perform preparation automatically. No `generate.go` file or
`go:generate` directive is required.

`Environment.Phase` is `assetbuild.PhaseGenerate` or `assetbuild.PhaseBuild`.
Each invocation must publish a complete usable asset set, including declarations
needed for compilation. Generation prepares real payloads because catalogs bind
or embed them; it does not emit placeholder assets. Phase alone does not change
an action's cache key; phase-specific processing must enter its declared options
or tracked dependencies like any other recipe choice.

Select a package or browser main-package pattern with
`engine assets generate ./samples/...`; `assets build` accepts the same selection.
With no selection, commands use the current project's recipe. Explicit
`--source assets --output generated/assets` still selects the default
image/mip/KTX2/catalog pipeline. A project without a recipe uses that default
when it has an `assets/` directory; otherwise preparation is a no-op.

`engine run web ./my-game --profile dev` and the sample gallery run recipes before
resolving generated imports. A recipe failure stops compilation. Recipes execute
on the host even when the game target is WASM. `dev` and `release` are the default
accepted profiles; set `Config.Profiles` to declare other names and use
`Environment.Profile` to select quality settings.

The recipe compiler clears inherited `GOFLAGS` and architecture-specific
settings and uses `CGO_ENABLED=0`. Compiling the recipe requires the pinned Go
toolchain. Selected processors may additionally require external build tools:
Materialize retains its CLI requirement; GPU preparation retains its device
and upstream wgpu-native library requirements. UV preparation and texture encoding/transcoding use
embedded WASM libraries through Go, without an executable installation.
See [asset tool ownership](../../spec/asset-tools.md) and the selected plugin's
guide. CGO-dependent recipe packages are outside this host execution profile.

`DefaultConfig` and directory-based image preparation use the named `textures`
pipeline. Their prepared filenames and bytes retain the image recipe contract;
the new graph namespace deliberately causes a one-time cache miss on upgrade.
Explicit legacy Config recipes keep their original cache identities.

## Plugins

The level pipeline uses the independent
[mesh cleanup and optimization stages](../mesh/build/README.md). Put cleanup
after prepared model import and before scene import; optimize after lightmap UV
preparation and before baking. The level adapter keeps geometry and lighting
data aligned. These offline steps add no mesh decoder to the game.

| Plugin | Configuration and behavior |
| --- | --- |
| `Images()` | PNG/JPEG import; infer albedo, normal, height, AO, SSBump and reflection roles from names |
| `ReflectionProbes()` | Bake `*-reflection` 2:1 panoramas to per-face mipmapped cubemap atlases |
| `Resize(ResizeOptions{...})` | Exact `Width`/`Height`, one dimension preserving aspect, or `MaxDimension` without upscaling; optional `Include` |
| `Materialize(MaterializeOptions{...})` | Run the Materialize v2 CLI on selected albedo images; append normal/AO resources while retaining albedo |
| `SSBump(SSBumpOptions{...})` | Bake linear RNM/SSBump coefficients and directional visibility from normal maps or authored grayscale heights |
| `Atlas(AtlasOptions{...})` | Explicit selected clamp-compatible groups; deterministic role-separated pages and extruded gutters |
| `Mipmaps()` | Linear-light albedo filtering, normalized normal filtering and linear AO/SSBump filtering |
| `KTX2()` | RGBA8 KTX2 by default; explicit `KTX2Options` selects offline BC1/BC7 with the supplied mip levels |
| `GoCatalog()` | Named embedded prepared KTX2 assets; page declarations and source regions for atlases |
| `ui/build.Images()` | UI-ready RGBA pages and a typed catalog; consumes regions from an optional preceding Atlas stage; no runtime source decoding |
| `ui/build.DefaultFont()` | Prepare and publish the qualified Go Regular atlas/metrics pair using Go and embedded msdfgen WASM |
| `model/build.GLBWithOptions(GLBOptions{})` | Flatten static GLB scenes into GPU-ready model resources and generated declarations |
| `impostor/build.Cards(Options{...})` | Capture prepared model resources into multi-view color/normal/depth cards and generated declarations |
| `level/build` plugins | Resolve scenes, partition static geometry, unwrap density-controlled UV1, bake RNM and publish external chunk assets |
| `model/build.PreparedGLB()` | Read ordinary source GLBs for later level/impostor steps without publishing the original model |

Select compressed image storage explicitly in the output stage:

```go
assetbuild.KTX2(assetbuild.KTX2Options{
    UsageFormats: map[string]texture.Format{
        assetbuild.UsageAlbedo: texture.BC1,
        assetbuild.UsageNormal: texture.BC7,
        assetbuild.UsageSSBump: texture.BC7,
    },
})
```

Here `texture` is `github.com/karty-game/karty-webgpu/pkg/texture`. Unselected
roles remain RGBA8; `Format` selects a common default for all roles. BC output
requires four-texel-aligned base dimensions and a device with BC compression.
It preserves dimensions, transfer and every supplied mip. BC1 thresholds alpha
at 128; BC7 retains smooth alpha. Encoding runs in embedded WASM during the build,
with no runtime decoder or automatic fallback. Direct offline callers can use
`CompressBC1` or `CompressBC7` on prepared RGBA8 KTX2. BC7 defaults to effort 2;
`Compression.LDRQuality` can explicitly select 0..4.

Model, impostor and image resources can coexist in one recipe. Image processors
preserve other resource schemas while rejecting image stages in the wrong order.
Put `model/build.GLBWithOptions(GLBOptions{})` before `impostor/build.Cards()`; the original model remains
available alongside its impostor. See [the impostor guide](../impostor/README.md)
for settings, runtime costs and supported materials.

Put `ReflectionProbes()` after `Images()` and before `KTX2()`. It accepts one
base-only 2:1 equirectangular PNG/JPEG whose logical name ends in `-reflection`
or `_reflection`, converts it to six horizontal cubemap faces, and builds each
face's mip chain independently to avoid filtering across face boundaries. The
prepared sRGB atlas is loaded with `texture/ktx2.Reflection`. This is an
environment-map bake from an authored panorama; the plugin does not capture or
render a 3D scene. See the [reflection sample](../../samples/reflections/) for a
complete recipe and placed probe.

Order is meaningful. Put resize before atlas and mipmaps before KTX2. An atlas
expects unmipped source images. Each source image has its own cached import and
transform actions; selected atlas groups become grouped actions. `Include`
patterns match extensionless logical identities relative to the input root, such
as `ui/buttons/play`. `**` spans directory components.

For example, insert this between resize and mipmaps:

```go
assetbuild.Atlas(assetbuild.AtlasOptions{
    Name: "ui", Include: []string{"ui/**"}, Clamp: true,
    MaxSize: 1024, Padding: 4, MipLevels: 3,
}),
```

`Clamp: true` is an explicit authoring assertion that selected assets can use
clamp sampling. It does not change runtime samplers. Standard/Source materials
currently repeat their maps, so retain standalone textures for those consumers.
Atlas pages need a consumer using the generated region bounds and suitable
sampling. No renderer changes are made by this package.

Atlas defaults are a 2048-pixel maximum page, one mip level and one-pixel gutters.
`MaxSize` must be a power of two no larger than 4096. Multiple mip levels require
padding of at least `1 << (MipLevels-1)`; packing aligns padded cells to that
scale. Mip generation honors this cap instead of mixing regions into a full
atlas tail. A region stores the image's dimensions after resize.

An atlas exports page functions and functions such as `UiButtonsPlayRegion()`.
Its returned `AtlasRegion` contains the prepared page `Asset`, pixel bounds and
literal `UVMin`/`UVMax` values. `atlas.json` exposes the same pixel mapping for
offline consumers. Original image names are not exposed as misleading full-page
texture functions. Apply geometry UV conversion during asset preparation.

## UI PNGs and automatic atlases

Use `github.com/karty-game/karty-webgpu/pkg/ui/build` (aliased `uibuild`) for UI
images. Keep each icon, frame and background as its own PNG source. The existing
Atlas plugin handles placement and extruded gutters; the UI plugin emits the
GPU-ready pages and named `*ui.ImageAsset` references:

```go
Output: "generated/assets",
Pipelines: []assetbuild.Pipeline{{
    Name: "ui", Input: []string{"assets/**"},
    Import: []assetbuild.Plugin{assetbuild.Images()},
    Process: []assetbuild.Plugin{assetbuild.Atlas(assetbuild.AtlasOptions{
        Name: "ui", Include: []string{"ui/**"}, Clamp: true,
        MaxSize: 512, Padding: 2,
    })},
    Output: []assetbuild.Plugin{uibuild.Images()},
}},
```

With pipeline `Input: []string{"assets/**"}`, a source `assets/ui/landscape.png` becomes
`UiLandscape` in the generated `assets.Catalog`. Call `images, err := assets.Load()`
once per asset lifetime and share that catalog; `images.UiLandscape` is directly
usable by `SetPanelBackground`, buttons or image nodes. Every generated page is
admitted once per Load, and all its source images share its pixel storage and GPU
texture. Source positions come from generated metadata, so changing packing does
not require application UV edits. Ordinary UI nine-slice borders remain explicit
source-image pixel insets, independent of atlas placement.

The plugin accepts standalone images too; omit Atlas when packing is unnecessary.
Optional resizing belongs before Atlas. UI output supports one straight-alpha
albedo RGBA8 level per page; do not add mipmaps, KTX2 or GoCatalog to this UI
chain. It publishes raw `.rgba` pages, generated `assets.go` and atlas region
metadata into the recipe output. No PNG decoding, runtime texture conversion or
runtime packing is added. Keep PNGs/recipe tracked and generated output ignored.
`engine run web` prepares recipes before resolving generated imports, including
on a clean checkout. See the [UI sample recipe](../../samples/ui/asset.config.go)
and [UI build plugin](../ui/build/README.md).

## Materialize and SSBump together

Select source albedo images to avoid colliding with authored normal/AO maps, then
compose the plugins before atlas packing or mip generation:

```go
Output: "generated/assets",
Pipelines: []assetbuild.Pipeline{{
    Name: "materials", Input: []string{"assets/*-albedo.png"},
    Import: []assetbuild.Plugin{assetbuild.Images()},
    Process: []assetbuild.Plugin{
        assetbuild.Materialize(assetbuild.MaterializeOptions{
            Preset: "brick", Maps: []string{"normal", "ao"}, Seamless: true,
            BackendIdentity: "my-gpu-and-driver-version/vulkan",
        }),
        assetbuild.SSBump(assetbuild.SSBumpOptions{Include: []string{"*-normal"}}),
        assetbuild.Mipmaps(),
    },
    Output: []assetbuild.Plugin{assetbuild.KTX2(), assetbuild.GoCatalog()},
}},
```

For `brick-albedo.png`, this produces the original albedo plus `BrickSSBump()`
and `BrickAO()` declarations. `KeepNormals: true` on SSBump additionally retains
`BrickNormal()`. Materialize emits independent normal/AO resource slots, so
unchanged output maps stop downstream invalidation even when the tool reruns.
Materialize selects logical IDs using the same role suffix conventions as
`Images`; custom producers must use those conventions and emit one standalone
albedo per selected resource.

The [material comparison recipe](../../samples/standard-materials/asset.config.go) has a
default path using authored normals and a `materialize` profile composing both
plugins. From the repository root:

```sh
mise exec -- go run ./cmd/engine assets build --project ./samples/standard-materials --profile materialize
```

Materialize resolves its module-owned published Materialize 2.0.0 package for
Windows amd64, macOS arm64 and Linux amd64/arm64 during the asset build. The
Windows binary requires the Microsoft Visual C++ runtime to be installed.
Mise's HTTP backend verifies the pinned HTTPS ZIP checksum, preserves
the shipped archive layout and notices, and reuses installed tools offline. Unsupported hosts fail
explicitly. It never compiles a native tool or installs anything during game loading.
`Executable` and tracked `MATERIALIZE_CLI` remain maintainer overrides.
The current adapter supports v2's normal and AO maps, emits OpenGL/Y-up normals,
preserves original albedo and validates generated PNG dimensions. Other Materialize map roles are not silently
reclassified. Preset, numeric overrides, seamless sampling and selected maps are
part of the action identity. `Environment` supplies explicit child environment
overrides; inherited backend/loader settings are allowlisted and tracked.

Materialize runs in a fresh staging directory with a two-minute default timeout,
bounded diagnostics and cancellation. Its verbose adapter report is checked.
Software adapters require explicit `AllowSoftware`; missing or unrecognized
adapter reports always fail. There is no automatic backend fallback.

Supply `BackendIdentity` to cache Materialize results. It must identify the
effective GPU, driver, linked libraries, loader configuration and backend; update
it when those change. The tool executable is fingerprinted automatically, but
its contents and environment path strings cannot identify changes to files at
those paths. Omitting the backend identity executes Materialize each time while
still allowing downstream content reuse. An explicit software identity can be
used for functional CI; it is not hardware graphics evidence.

SSBump defaults preserve the former Source sample bake: 300 hemisphere rays per
texel, 32-texel visibility distance and 0.05 height bias. `Rays`, `Distance` and
`Bias` are bounded overrides; zero selects the defaults. Sources must be square,
power-of-two normal maps from 2 through 512 pixels and fit the total ray-work
limit. Resize before baking when necessary. Relief is estimated by periodic
normal integration; this is not a scene lighting bake. SSBump replaces normal
images by default, preserves other image roles and retains resource grouping IDs.
Names/sources ending in normal become ssbump; downstream selection still uses
the original grouping ID. Mips average the linear coefficients directly.

Authored `*-height` / `_height` images are linear grayscale build inputs. The
same SSBump plugin derives tangent normals from periodic central differences and
traces the authored heightfield directly, reusing its RNM projection and
visibility integrator. `HeightScale` is the full grayscale relief range in
output texels (default eight; finite positive values up to 512). Resize heights
before this stage; the image plugin filters them linearly. The same dimension,
ray-work, cancellation and cache bounds apply. Height sources are consumed even
with `KeepNormals`; they cannot be exported as runtime KTX2 textures.
The [Sponza recipe](../../samples/sponza/asset.config.go) composes this path with
the standard GLB, chunk, UV, production RNM bake and portable-publication
modules. Its one source model retains opaque and masked parts; shared build
modules perform separation and native texture preparation. The sample contains
no source-processing tool directory or extra recipe package. This adds no
runtime conversion or shader.

## Automatic caching

The runner stores immutable records and content-addressed blobs under
`generated/cache/assets/v1/`. It hashes source contents, processor revision, effective
options, declared input slots and tracked file/glob/environment dependencies.
The actual recipe executable fingerprints code, including local plugin edits.
The CLI uses `go run -trimpath` so Go also caches the linked recipe executable;
unchanged recipes do not relink into a new temporary binary on every invocation.
That deliberately invalidates all recipe action candidates when code changes;
it also invalidates broadly when literal config options change the binary.

An unchanged run still discovers and hashes inputs, but skips processing.
Downstream actions depend on individual output content hashes: a PNG edit that
decodes to the same pixels can stop after import. Catalog generation depends on
names and mappings separately from texture bytes. Deleting generated files can
restore them from cache without rerunning processors. `--no-cache` executes all
actions without reading/writing persistent action-cache entries. Auxiliary
artifacts used to connect discovered jobs still use the project-confined blob
workspace during that invocation; they do not permit action reuse.

All final files, including the ownership manifest, preserve their modification
times when identical. When the owned output roster and every file's contents
match the prepared results, publication skips serializing and writing the
recovery journal. Missing or edited files still take the journaled repair path.
Processing completes before publication. A project lock
serializes publishers; an interrupted publication journal is recovered before
the next build proceeds. Source/output/cache overlap, path escapes, symlinks and
unowned output replacement are rejected. The cache is disposable and is not a
required build input.

The lock and publication journal also live under `generated/cache/asset-build/`.
Recipes may publish into `generated/` itself; `cache/` is reserved and cannot be
emitted, replaced or pruned as a generated asset. Deleting `generated/` removes
both prepared outputs and local processing state; the next build restores them
from declared sources. The builder does not create or use the CLI's `.karty/`.

Actions execute serially by default. Explicit build concurrency is described below.
Publication journals use compact version 2
metadata that references SHA-256 addressed binary files in the project's
`generated/cache/asset-build/publication-blobs/` directory. Every staged payload,
action output and journal remains bounded at 256 MiB. Recovery checks the complete
roster, paths, sizes and hashes before writing outputs. The journal is removed
before its referenced staged files are cleaned; unrelated staged hashes and the
action cache remain untouched. Version 1 journals with embedded JSON payloads
remain recoverable.

## Explicit build concurrency

Use `engine assets build --project ./my-game --jobs 2` to permit independent
processors to overlap. Omission, zero and one retain serial execution. Every
recipe form uses one scheduler, including upfront static graph admission before
processor callbacks; malformed references, slots and cycles fail before execution.
Only
`Action.Parallel` callbacks opt in; custom plugins must own their mutable state
and preserve results independently of scheduling. Share an opaque pointer from
`NewExecutionResource()` through `Action.Exclusive` to serialize a native helper
or another owned resource. Required processing order still uses input Refs.

The production lightmapper uses one CPU-support token and one native-worker
token, permitting support preparation alongside tracing. Other processors retain
their serial action policy. A branch discovery boundary waits for its own preceding work without draining unrelated pipelines. Serial
actions remain global barriers. `Options.Jobs` counts actions, not
threads or memory; native tools retain their existing thread settings. Concurrent
admitted work increases resident memory: the measured Sponza Jobs2 comparison
saved 17.884 s but increased GNU time maximum RSS by about 1.00 GiB. Default behavior
is unchanged. See the [versioned execution contract](../../spec/asset-pipeline-v2.md)
for resource limits and error precedence; the under 60 s full-bake goal is unmet.

Use optional `Options.Trace func(ActionEvent)` for structured start/finish times
including cache admission. It runs on the coordinator, must return promptly and
must not mutate the plan. Trace completion order can differ from the stable
`Report.Actions` order. Tracing adds no default stdout messages. Full planning
admission precedes callbacks in opt-in regions; runtime failure selection,
cleanup and failed-build publication remain deterministic and bounded.

## Custom processors

A `Plugin` implements `Name() string` and `Plan(plan *assetbuild.Plan) error`.
Planning adds `Action`s with named output `Slot`s and explicit input `Ref`s.
Actions return immutable `Result` byte records; the runner owns cache persistence.
`Plan.Resources` connects logical assets to their current records. A published
slot contains `EncodeBundle(Bundle{...})` bytes mapping relative filenames to
payloads. Binary bundle version 1 stores raw bytes with sorted filenames, bounded
lengths and a checksum, avoiding base64 expansion while retaining the 256 MiB
complete-record limit. Names are bounded at 4096 bytes and binary bundles at
65536 files. Existing processors may continue publishing legacy JSON bundles.

Use `Context.Input`, `ReadFile`, `Glob` and `Getenv` for action dependencies.
`Context.TrackTool` resolves an executable and tracks its contents, permissions
and resolution, including PATH changes and symlink retargeting. Pass its returned
absolute path to `exec.CommandContext`, and declare tool inputs, relevant
environment and external library/backend identity separately.
Handle cancellation through the embedded context. Plugins using untracked state
must select `CacheDisabled`; their downstream results still use content hashes.
Recipes are trusted Go programs: the runner cannot intercept arbitrary `os`
calls. Use explicitly declared inputs or disable caching for processors with
hidden dependencies.

## Manifest-driven jobs and auxiliary artifacts

A cached discovery action can call `Context.StoreArtifact(data)` for each
bounded prepared payload and return a small manifest of `Artifact{SHA256, Size}`
references. No combined level-sized record is required. Each artifact retains
the 256 MiB admission ceiling and lives under the project cache's `artifacts/`
directory. These per-record bounds do not cap total host memory to one artifact;
the runner retains action outputs and final publication payloads until completion. Stored and read artifacts participate in dependency validation;
missing or corrupt blobs invalidate cached producers and are rebuilt normally.
Deleting the cache remains safe because producers consume declared source data.

`Plan.Expand(Expansion{ID, Inputs, Run})` discovers jobs after its declared input
actions complete. Its callback receives a private child plan and may add actions,
publish their outputs, update resources and register nested expansions. The
runner commits that graph only after the callback succeeds. IDs are unique within each named pipeline (global in legacy recipes). Nesting is
capped at 32 levels and the complete graph at 65,536 action and expansion
identities. Later expansions see resource changes from earlier
ones in the same pipeline. All processing still finishes before the normal atomic publication.

`Resource.Process` optionally lets a resource owner plan operations for compound
records. Geometry stages call it with `mesh-cleanup` or `mesh-optimize`; the owner
registers normal actions/expansions and returns the current or replacement Ref.
Deferred discovery installs its replacement before the next recipe stage.
The callback is build metadata, excluded from JSON and runtime assets. Its
actions still declare their processor revisions, inputs, options and artifacts.

Expansion callbacks perform cheap discovery on `Context.Input` manifests and
verified `ReadArtifact` payloads reachable from those declared inputs. Source,
environment, tool and artifact-write operations are rejected during expansion.
Expensive work belongs in the discovered cacheable actions.

Declare stable auxiliary inputs with `Action.Artifacts: []Artifact{...}`. Their
hashes and sizes enter the action identity independently of the complete discovery
manifest. A membership change can then add or remove chunk jobs while unchanged
chunks keep their cache hits. `Context.ReadArtifact` accepts an explicitly
declared artifact or one reachable through ordinary declared input actions;
undeclared access fails. Payload reads are independent owned snapshots.

`Plan.Cleanup(func() error)` registers a build-scoped finalizer. Finalizers run
once in reverse order on success, processing failure, cancellation and partial
planning failure, with errors joined to the build result. A failed expansion
immediately closes its newly registered finalizers. A lazy GPU or tool session
can register its close function before allocating, so a cache-only build never
needs to open the session.

See the [design specification](../../spec/asset-pipeline-v1.md) for the broader
contract and acceptance scenarios.

The [level build guide](../level/build/README.md) composes scene/material import,
StaticChunks, LightmapUVs with production preparation, ProductionBake and
PublishVariants. Chunk/page discovery uses bounded
manifest expansion and independent artifact inputs; geometry processing remains
in cached actions. The [runtime level loader](../level/README.md) consumes external
GPU-ready geometry, shared textures and completed directional lightmaps.

Build-only LDR lightmap exports can normalize a complete RGB/RNM family with
`PrepareLDRLightmapCoefficients`, prepare opaque color once with
`PrepareOpaqueLDRUASTC`, then export BC1, BC7 or ETC2RGB with
`TranscodeLDRNative`. The returned shared physical scale restores the light's
units; Linear/SRGB transfer and authored mips are retained. The same native
export accepts prepared numeric SSBump and bounded opaque ETC1S albedos.
[Codec profiles and portable library requirements](block_compress_tools.md) describe
the quality settings, admission and canonical container contract.

`CompressBC1` converts prepared RGBA8 mips to BC1 RGBA blocks using embedded
WASM and an offline Go coverage pass. Source alpha ≥128 becomes opaque; lower
values become transparent. It preserves dimensions, authored mips and
linear/sRGB transfer without resizing or runtime conversion. Base dimensions
must be multiples of four. The impostor builder uses this for both its
color/coverage and packed normal/depth atlases; see the
[impostor storage contract](../../spec/decisions/0020-bc1-impostor-atlases.md).


### Portable libraries and managed native dependencies

Texture codecs, UVs and fixed Go Regular font preparation use embedded WASM
executed by Go/wazero.
Texture/UV executable options and native qualification paths are removed. No native executable,
Node runtime, source compiler or Karty release installation is required for those
operations. One lazy Basis session belongs to each asset build and reuses its
encoder/transcoder instances; direct standalone codec calls own temporary
sessions. Existing action revisions and recipe identity distinguish the new
implementation from cached native results. See [Decision 15](../../spec/decisions/0015-portable-offline-libraries.md).

Remaining native modules use `Context.ManagedTool(ManagedToolSpec)` to delegate
a module-owned pinned package to mise's HTTP backend. `Context.TrackTool` tracks
explicit executable paths; the retired codec/font alias registry is removed. Ashura records the actual
executable and loadable-component content as action dependencies. There is no
per-project installer or game-time tool preparation. Materialize retains its
explicit CLI selection; this provider does not select texture or UV encoders.
See [managed tool provider revision 2](../../spec/asset-tools.md#managed-build-tool-provider-revision-2)
for supported hosts, cancellation, dependency tracking and pending native
releases. Mise's `MISE_DATA_DIR` and `MISE_CACHE_DIR` control installation/cache
placement; the former `KARTY_ASSET_TOOL_CACHE` installer is retired.

Offline importers with embedded source images can call `ResizeImage(image, ResizeOptions)`
before generating mips. It uses the same filtering and bounds as Resize, accepts
one base-only image, and rejects a pipeline Include selector. Model recipes expose
this through `modelbuild.GLBWithOptions`; source files remain immutable.

Native libraries used by offline processors can use `Context.ManagedLibrary`
with the same pinned archive specification as `ManagedTool`; `Executable` names
the primary library file for this operation. The existing mise provider owns
installation and checksum verification. `TrackLibrary` records library content
and resolved targets without requiring executable permissions. This explicit
external dependency does not relax project source path confinement or install
anything during game loading.
