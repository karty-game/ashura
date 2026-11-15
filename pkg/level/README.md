# Baked levels

Schema v7 supports calibrated `RGBDiffuse` pages (one map) and `RNMFit` pages
(three maps). `preview` marks unqualified lighting output. See the
[v6 calibration contract](../../spec/baked-level-v6.md) and
[v7 LDR delivery](../../spec/baked-level-v7.md).

V7 prepares sRGB BC7/BC1 or ETC2 RGB lightmaps offline, retaining one shared
energy scale per complete RGB/RNM atlas tuple. BC profiles use linear BC7
SSBump; ETC2 profiles preserve exact linear RGBA8 SSBump and require no ASTC
capability. The default `NewLoader` includes no runtime decoder.
Select the descriptor with `TextureVariants.Select` after device initialization,
before manifest I/O.

Native Linux uses the same prepared variant selection as browser WebGPU. Device
initialization enables supported compression families, and the native uploader
consumes the selected GPU blocks directly, including their authored mip chain.
An unavailable family is an explicit error; loading never expands it to RGBA.

Legacy ETC1S callers explicitly use `level/basis.NewLoader` or inject a
transcoder through `NewLoaderWithTranscoder`. For that opt-in delivery, see [baked level v3](../../spec/baked-level-v3.md).
Its web worker transcodes to compressed GPU blocks during `Request.Read`, with
no RGBA expansion. Use `Loader.Request` with an active device. The current native
backend and standalone `ReadChunk` reject that opt-in profile; existing assets
keep their current behavior. RNM and SSBump retain their numeric encoding.

A baked level contains a small manifest, independently loadable static chunk
geometry, shared Source material textures and three RNM lightmaps per page.
Level preparation is build-only. The runtime consumes prepared bytes; it never
clips or merges geometry, unwraps UVs, prepares transport or repacks textures.

The first adapter consumes the existing strict scene YAML/prefab format. Meshes
are static by default. `render3d.model.isDynamic: true` keeps a placement separate.
Static geometry is clipped into configurable grid cells within its logical space,
then merged by compatible material. Logical object/save identities remain alive
independently of render residency. Generated factories use `NewBakedModel` for
static placements and ordinary `NewModel` for dynamic placements, so supplying a
shared dynamic model cannot also instantiate its merged static source.

```yaml
version: 1
root:
  id: room
  children:
    - id: architecture
      components:
        render3d.model: {asset: assets/room.glb}
    - id: door
      components:
        render3d.model: {asset: assets/door.glb, isDynamic: true}
```

The [build plugins](build/README.md) prepare UV1 at physical texel density, gather
lighting through the shared offline/runtime RNM shader and publish external
payloads. Direct, environment and one-bounce diffuse GI share the same RNM maps;
Source SSBump coefficients apply to the combined result.

## Runtime consumption

For an explicitly selected prepared scene, `level.Open` owns the ordinary
asynchronous reads, stepped loading/presentation and cleanup. Pass native profile
variants, an explicit public asset base and a required selection policy; it
starts I/O and selects the device profile on the first `Update`, after App.Run
opens the device. Register `presentation.Update` with `App.OnUpdateError` and own
`presentation.Close`. `Status()` returns stored chunk/object counts plus Ready
and Closed; optional StatusChanged updates retained UI only after progress.

Closing from an App/UI browser callback safely cancels and joins pending reads,
including standard Go WASM. The reader detaches Go callbacks immediately on
cancellation; later Fetch/stream/WebCrypto settlement cannot reenter an exited
WASM application. Integrity checks and App-loop resource ownership are unchanged.
`level.WholeScene` explicitly selects one authored single-space scene and its
build-flattened independent models. Portal worlds retain their own policy.
See the [V1 presentation contract](../../spec/level-presentation-v1.md) for
selection, cancellation, external-binding errors and lifecycle guarantees.
`resource.ProjectBase(project)` selects the standard native `generated/public`
directory or the browser export's relative asset root without sample platform
files.

Fetch a generated descriptor using the existing resource reader. Chunk selection
belongs to the application/world streaming layer; grid proximity does not imply
portal reachability or adjacency.

```go
// Read the small manifest on an I/O task.
manifest, err := level.Read(ctx, publicBase, levels.Room())
// Handle err. Choose chunks using world visibility/streaming ownership.

// On the App loop, snapshot resources already resident for this chunk:
request, err := loader.Request(manifest.Chunks[index])
// Handle err. On an I/O task:
prepared, err := request.Read(ctx, publicBase)
// Handle err. Back on the App loop:
task, err := loader.Begin(prepared)
// Handle err. Call once per update until done:
done, err := task.Step()
if done && err == nil {
    chunk, resultErr := task.Result()
    // Handle resultErr, then:
    instance, presentErr := chunk.BeginPresentation(world)
    // Handle presentErr. On subsequent updates, call once per update:
    visible, presentErr := instance.PresentNext()
    // Handle presentErr. Keep the instance after visible becomes true.
    _ = visible
    // Later: instance.Remove(), then chunk.Unload().
}
// Cancel pending work explicitly if it is no longer wanted:
err = task.Cancel()
```

Create one `level.NewLoader(app)` per residency domain before using these calls.
Its requests snapshot only the selected chunk's resident texture metadata. Shared
resident images are neither downloaded nor decoded again. If they are evicted
while the I/O task runs, `Begin` returns `ErrStaleRequest`; request the chunk again.
Pending tasks pin acquired resources until completion or cancellation.

Each `Step` creates and uploads at most one prepared texture, or admits one
geometry page. `App.PrepareTexture` uses the ordinary backend upload/cache before
that image's first visible draw, preventing all image uploads from bunching at
chunk visibility. It adds no source conversion, repacking or GPU completion wait.
Before App.Run it retains ordinary first-use upload behavior. A 1024 × 1024 RGBA8 image
with its full mip chain is approximately 5.33 MiB; this is a bound on payload size,
not a guarantee of device upload latency. Record frame gaps on target hardware.
A 2048px RNM map is 16 MiB. Current whole-texture and whole-page steps are too
coarse for smooth loading of the consolidated Sponza profile: the measured
candidate reduces draw cost but still has loading stalls. See
[validation](../../spec/validation.md); larger atlases do not establish smooth
streaming by themselves.

After loading, `BeginPresentation` retains the chunk without adding draw records.
Call `Instance.PresentNext` once on each following update. Each call exposes at
most eight existing prepared material ranges from one lightmap page, bounding
first-draw geometry/bind-group creation. It never subdivides geometry or rebuilds
chunks. Partially visible geometry can be removed immediately with `Remove`;
failure rolls the entire partial presentation back. Pending presentation prevents
chunk unload, including before its first range is visible. Completed calls are
idempotent, and App close removes partial presentations.

`ReadChunk`, synchronous `Loader.Load` and synchronous `LoadedChunk.Instantiate`
remain convenience APIs. Use resident-aware requests, stepped loading and stepped
presentation for interactive streaming. These bound work units, not wall-clock
latency or physical device FPS. Resource
SHA256 checks remain mandatory; browsers use asynchronous WebCrypto before
copying verified bytes into WASM, while native readers retain the Go hash path.
Opaque verified payloads carry the reader's checksum and immutable ownership
through model creation. The loader avoids a second model hash and vertex/index
copy. Private received texture storage is adopted into App handles without
another full pixel copy; public `App.NewTexture` still copies caller buffers.
`App.AdoptTexture` is an explicit ownership contract: callers must relinquish
mutation of pixel storage. These paths do not add per-frame byte/time quotas or
change the caller-controlled loading steps. Browser upload staging and GPU
driver costs remain measured limits, documented in validation.

Keep one Loader per residency domain to share texture handles across chunks.
Unload rejects live instances and releases shared textures only after their last
owning chunk is gone. App close releases the loader's resources. Load and all
world/resource mutation belong to the App loop. The loader has no camera scan,
LOD scheduler, per-frame geometry processing or implicit whole-level preload.

The prepared manifest is `karty.baked-level@1` (RNM up to 1024px) or
`karty.baked-level@2` (RNM up to 2048px). V1 retains its original bounds and older
readers reject v2 before fetching resources. Both record stable cell IDs and
logical spaces, chunk origins/bounds/source identities, geometry/UV references,
material texture references, page dimensions, three-map families, encoding and
energy scale. Payload references include relative paths, exact byte sizes and
SHA256. Lightweight `KARTUV1` layouts bind UV1 to the exact localized, texture-free
prepared geometry. Model, UV and image metadata are admitted before resource
creation. RGBA8 RGBE maps are decoded before bilinear interpolation in Source.

Source YAML, GLBs, xatlas data, receivers, BVHs and diffuse transfer arrays are
not production runtime dependencies. Optional developer output includes bake
inputs for development tooling. Static pages are fixed to the authored placement;
moving a merged object requires a new build generation. Use dynamic placements
for geometry that moves during gameplay.

## Current scope

V1 supports opaque Source world materials, static point lights, explicit
sky/ground illumination and zero/one diffuse bounce. Authored SSBump overrides
are preserved; prepared normal maps can derive SSBump offline. Unsupported
material features fail explicitly. Albedo/SSBump payloads must fit the 1024-pixel profile;
material sampling retains 8x anisotropy. The profile is deliberately versioned.
Production publication consolidates baked UV islands and merges compatible
material ranges within each final chunk/atlas. Small offline bake jobs do not
force small production pages. There is no material-image atlas or texture-array
consolidation. The current Sponza candidate has two chunks, two 2048px atlases
and 43 ranges, replacing the initial 415-range output. Its unchunked source has
22 ranges; materials still repeat across chunks. The candidate passes resource
checks and improves steady draw cost, but has not passed smooth-loading acceptance.

Each logical visibility scene and page respects the existing 128 MiB portable GPU
buffer limit. The format does not make arbitrarily large worlds fit one GPU
binding. Partitioned acceleration structures, automatic portal streaming, dynamic
model irradiance probes, reflection captures, automatic impostor selection and
editor watch/hot reload are subsequent work. The production compute baker uses
the external native WebGPU CLI worker; it does not launch Chromium. The legacy
V1 sample baker retains its separate browser workflow.

The build report separates action processing from publication. Measure chunk
load/update gaps and peak residency on target devices; successful desktop baking
and browser execution do not establish mobile thermal or frame-pacing readiness.
