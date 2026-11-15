# Prepared lightmap baking

The Source RNM path gathers direct light, environment and one diffuse bounce into
three directional maps, evaluated together by Source SSBump materials. See the
[research and format decision](../../spec/source-rnm-v1.md). The retained scalar
baker is described below. These are historical interactive APIs. Maintained
offline asset builds use [the production API](bake/README.md), production UV
preparation and the in-process Go WebGPU baker.

Prepare inputs with `LightmapOptions{RNM: true, PrecomputeTransport: true,
TransportSamples: 64}`. UVs, receiver rasterization, BVH, surface reflectance,
environment visibility and patch transfers are build-only work. Runtime changes
light values against that fixed geometry.

```go
baker, err := lightmap.NewRNM(app, inputs)
// Handle err. Declare resources before app.Run.
err = baker.SetScene(lightmap.RNMScene{
    Points: []lightmap.Point{{
        Position: [3]float32{0, 2, 0}, Color: [3]float32{1, .75, .45},
        Intensity: 5, Range: 20,
    }},
    SkyColor: [3]float32{.12, .14, .18},
    GroundColor: [3]float32{.025, .02, .015},
    Samples: 8, Bounces: 1, RowsPerRecord: 32,
})
sourceMaterial.RNMLightmap = baker.Lightmap()
app.OnCompute(func(frame *engine.Frame) error { return baker.Record(frame) })
```

`Samples` controls the v2 ray-based fallback; prepared v3 inputs retain their
build sample count. `Pending` and `Progress` report row scheduling, not GPU
timestamps. Maps, scale and encoding publish together at final resolve. With
unchanged light settings `Record` does no work. Remove baked lights from the
live material contribution to avoid double counting.

The old `engine assets lightmap` command is retired. It returns migration
guidance before reading files or starting an executor, including when passed
`--browser` or `--rnm`. Use `LightmapUVs(Production: true)` and
`ProductionBake` in an asset recipe, then run `engine assets build`. Existing
`.klightmap` artifacts use a different receiver and RNM calibration contract;
production baking requires explicit preparation from source rather than an
automatic conversion of those artifacts.

Historical RNM outputs record scale, encoding, scene values, checksums, adapter
and wall time. V3 uses RGBE8 in linear-space RGBA8 textures; decode exponents
before interpolating. `texture.RNMLightmap` carries this encoding to the Source
shader. Existing output readers and publication admission remain compatible.

## Retained scalar direct-light API

This package loads build-prepared lightmap inputs and computes direct diffuse
illumination from one point source. Offline execution and runtime baking use the
same complete WGSL file, `internal/lightmapkernel/wgsl/direct.wgsl`.

The build-only `model/build.LightmappedGLB` plugin unwraps and remaps model
geometry, prepares UV1 streams, a triangle BVH, receiver positions/normals and
chart-local gutter references. See the [Sponza sample](../../samples/sponza/) for
its recipe. The resulting `.klightmap` is bound to the exact prepared model's
SHA-256. Loading verifies the version, complete header/payload checksum, array
bounds, finite values, UV ranges, padding references and BVH tree/depth before
declaring GPU resources. It performs no charting, geometry conversion, receiver
rasterization or BVH construction.

Prepared inputs support atlas dimensions through 2048, at most 4096 primitive UV
streams, 1,048,576 vertices/triangles/BVH nodes and 256 MiB total encoded bytes.
The selected GPU must also admit each storage binding; a 2048-square receiver
buffer alone occupies 128 MiB. Smaller pages are preferable for interactive
experiments. The sample uses a single atlas rather than a chunk streaming system.

```go
inputs, err := models.AssetsSponzaLightmap().Load()
// Handle err, then load the matching remapped model and UV streams.
baker, err := lightmap.New(app, inputs)
// Handle err. Resource declarations happen before app.Run.
err = baker.SetPoint(lightmap.Point{
    Position: [3]float32{0, 2, 0},
    Color: [3]float32{1, .75, .45},
    Intensity: 5, Range: 20,
})
app.OnCompute(func(frame *engine.Frame) error { return baker.Record(frame) })
```

`SetPoint` requests another bake only when source settings change. `Record`
submits one complete atlas dispatch and otherwise does no work. It records
before rendering, performs no readback or GPU completion wait, and does not
create resources after startup. `Recordings` counts recorded dispatches; it is
not GPU timing or evidence that presentation completed. The first slice does not
split a full atlas dispatch into a progressive background schedule.

The kernel uses squared range falloff `(1-distance²/range²)²`, Lambert diffuse
and double-sided triangle visibility. Double-sided receivers use the absolute
normal/light dot product. Zero bias chooses .015 model units. Receiver shading
uses authored interpolated normals; visibility origins move toward the light
to avoid offsetting along a smoothed normal into adjacent surfaces.

Output is a one-mip linear RGBA8 texture. RGB stores diffuse irradiance divided
by `baker.Scale()`; alpha is one for receivers/gutters and zero elsewhere. Restore
the scale after sampling and multiply by albedo sampled through UV0. UV1 does
not inherit material tiling. Covered black remains a valid shadowed surface.
Gutter texels directly evaluate their build-selected receiver: no runtime CPU
image processing or dilation pass is needed. No unrestricted mip generation is
safe for this atlas profile.

The bake contains neither albedo, ambient fill, bounced light nor specular.
Disable the corresponding live direct contribution when showing its baked
result. Source positions are in prepared-model coordinates. Changing the model
placement is not supported by this first bake profile.

## Historical offline compatibility APIs

`lightmap/build.BakeBrowser`, `BakeRNMBrowser` and `NewRNMSession` are deprecated
explicit compatibility APIs. They retain their historical browser execution
behavior for callers that deliberately invoke them. They are not supported asset
build paths: the CLI cannot launch them, and the inert `level/build.RNMBake` /
`BakeOptions` facade has been removed. Level recipes use `ProductionBake` with
production UV preparation; their current build package no longer imports the
historical browser executor through retired options.

These compatibility functions use an isolated browser, token-protected loopback
routes and the historical shared runtime shader. Their output contract and
limitations remain distinct from calibrated production lightmaps. New offline
work belongs in [the native production API](bake/README.md); missing native tools
or hardware never select a browser fallback.

## Historical browser compatibility fixture

```sh
KARTY_LIGHTMAP_GPU_TEST=1 KARTY_LIGHTMAP_BROWSER=/path/to/chromium \
  mise exec -- go test ./pkg/lightmap/build \
  -run TestHardwareDirectVisibilityAndPadding -v -count=1
```

The opt-in fixture checks a blocked receiver, the analytic unblocked diffuse
value, HDR scale, chart padding and an empty texel using actual GPU readback.
Ordinary tests exercise malformed-input admission without requiring a GPU.

## Historical session lifetime

Explicit compatibility callers can use the deprecated
`lightmap/build.NewRNMSession(ctx, BrowserOptions{...})`
once, call its `Bake(jobContext, inputs, scene)` for each prepared page, then
`Close`. The session retains one hardware browser, GPU device and shared WGSL
pipeline set. Jobs are sequential; buffers and textures are released after each
completed readback. A canceled in-flight job invalidates the worker, preventing
stale results from reaching a later page. Construct the session with the build
lifetime context and pass per-job deadlines to `Bake`.

Session result duration measures that job's uploads, execution and readback;
it excludes worker startup. It is wall time, not a GPU timestamp. Cache outputs
only with an explicit effective GPU/driver/backend identity, as with other
hardware-dependent build actions. Offline output remains byte-identical to the
one-shot executor for the same admitted inputs and settings.
