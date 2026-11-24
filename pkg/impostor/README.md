# Prepared model impostors, format v4

**The comparison uses screen-size adaptive octahedral reprojection: one quad,
three views and six to twelve texture reads per fragment.** It blends albedo and normals at
matching corrected coordinates, writes reconstructed depth, and uses ordinary
filtered BC1 image mips. The layered volume tracer remains an explicit
reference, with better geometric reconstruction but much higher fragment cost.

`impostor/build.Cards(Options{})` consumes a prepared static model or flattened prefab.
Rasterization, material sampling, texture packing and mip generation happen offline
through the cached asset pipeline. Runtime
resources arrive GPU-ready. This is an independent implementation; it does not
require a Blender add-on.

The public zero-value `Cards(Options{})` configuration selects octahedral mode for
compatibility. The [comparison sample](../../samples/impostors/) explicitly selects
octahedral mode at a temporary 256px, a 7×7 frame grid, padding 8 and three mips
for its `dev` and `release` profiles. It retains 49 stored views and three-view
blending; a six-view redesign is deferred. Public zero-value layout defaults
remain 8×8/80px.

## Build and use

Put the GLB importer before the impostor plugin in `asset.config.go`:

```go
import (
    "github.com/karty-game/ashura/pkg/assetbuild"
    impostorbuild "github.com/karty-game/ashura/pkg/impostor/build"
    modelbuild "github.com/karty-game/ashura/pkg/model/build"
)

// Inside assetbuild.Main's configuration function:
return assetbuild.Config{
    Output: "generated",
    Pipelines: []assetbuild.Pipeline{{
        Name: "models", Input: []string{"assets/*.glb"},
        Import: []assetbuild.Plugin{modelbuild.GLBWithOptions(modelbuild.GLBOptions{})},
        Process: []assetbuild.Plugin{impostorbuild.Cards(impostorbuild.Options{
            Mode: impostorbuild.Octahedral,
            Resolution: 256, FrameGrid: 7,
            Padding: 8, MipLevels: 3,
        })},
    }},
}
```

Run `engine assets build --project ./my-game`. Generated `models` declarations
load the original; generated `impostors` declarations load the replacement:

```go
card, err := impostors.AssetsTree().Load(app)
// Handle err.
instance, err := card.Instantiate(world, engine.DefaultTransform())
// Handle err.
instance.Remove()
err = card.Unload()
```

Use the generated function name for your source path. The GLB plugin's original
model bundle remains available; the impostor action consumes its cached prepared
model without compiling the GLB again. `impostorbuild.Bake(ctx, preparedModel,
options)` returns container bytes for custom build tools. `BakeWithReport` also
returns the maximum interval count and any tolerated negative-winding pocket
count/depth. Games import `impostor`, never its build package.

## Adaptive octahedral rendering

Camera/object direction selects three neighboring captures and barycentric
weights in the vertex shader. The grid diagonal mirrors with the octahedron's
quadrants. The sample uses seven frames per axis so poles, axis boundaries and
fold lines align with capture-grid vertices/edges.

Each view uses zero, one or two depth-based UV corrections, then samples
color/coverage and normal/depth together at the final coordinate. That costs
6/9/12 texture sampling instructions across three views. The second lookup uses
a bounded secant refinement only when the two depth residuals bracket a root;
it stays inside the seed-to-first-correction segment. Otherwise it retains the
first projection. Blindly repeating a depth correction can introduce holes.
The two correction
branches are explicitly bounded; there is no ray march, hierarchy search,
nearest-frame switch, temporal cache or CPU camera-distance scan.

`Asset.Load` initializes `DefaultQuality()`. It estimates projected sphere
size from GPU derivatives and blends correction strength through these bands:

| Approximate sphere diameter | Corrections | Texture reads |
| --- | --- | --- |
| ≤16px | 0 | 6 |
|16–32px | Smooth 0→1 | 9 |
|32–128px | 1 | 9 |
|128–256px | Smooth 1→2 |12 |
| ≥256px | 2 |12 |

Mip selection independently follows unwarped projected UV derivatives. Bias 0
uses the normal footprint, positive bias samples coarser mips, and negative bias
samples finer mips. Filtering may read several texels per sampling instruction.
The shared atlas remains resident; smaller mips do not free GPU memory.

The temporary sample setting stores 49 captures at 256px /7×7 /padding 8 /three
mips, with 256→128→64px views. Each of its two BC1 atlases has levels of 1904×1904,
952×952 and 476×476 texels. Together they contain **4,758,096 block payload
bytes (4.54 MiB)**, or 14,274,288 bytes (13.61 MiB) for all three sample pairs.
The static descriptor is 3,168B and geometry 152B.
Container metadata and executable/host files are additional; these figures are
not complete sample export sizes. Retained CPU copies and GPU allocation overhead
are separate residency costs. The published sample uses standard Go with a
35,000,000-byte per-platform budget to preserve the temporary 256px captures
and all three reference models. See the [comparison sample](../../samples/impostors/README.md).
The `detailed` profile is an extended-mip reference at the
same 256px capture resolution, with padding 16 and five mips. All capture, dilation, mip construction,
channel packing and compression are offline; camera motion causes no asset rebuilding.

Both atlases use BC1 RGBA blocks. Color is sRGB with one-bit silhouette coverage:
the build retains alpha values ≥128 as covered and clears the others in each
authored mip. The linear numeric atlas stores octahedral object-normal coordinates
in R/B and normalized capture depth in G, using the RGB565 green channel for
depth. Its alpha stays opaque. GPU sampling decodes the blocks; the shader decodes
the oct normal and depth channels and compensates for BC1's transparent-black
filtering before blending views. Quantized color, normals, depth and coverage
can introduce visible artifacts. The active GPU device must support BC compression;
there is no runtime decompression, recompression or alternative texture fallback.

## Quality controls

Use `SetQuality` on the owning App loop, after loading and independently of the
asset recipe. Settings are shared by all instances of this loaded resource:

```go
quality := impostor.DefaultQuality()
quality.FarPixels = 16  // First correction fades in over 16..32px.
quality.NearPixels = 128 // Second fades in over 128..256px.
quality.MaxCorrections = 2 // Cap at 0, 1, or 2.
quality.MipBias = 0 // +1 samples one mip coarser.
err := card.SetQuality(quality)
```

Set `Adaptive:false` for fixed 0/1/2 correction references, costing 6/9/12 reads.
These controls change reprojection work; they cannot restore capture resolution,
BC1 detail or one-bit silhouette coverage. `Quality.Validate()` checks settings without changing
an asset. Near must be at least twice Far, Far must be positive, Near ≤32768,
and mip bias must be finite in [-4,4]. Invalid edits leave the last valid state
intact. `Quality()` reads settings; `SupportsQuality()` reports octahedral support.
Other capture modes ignore this policy.

Settings use four existing material-uniform lanes. A shared CPU settings object
allows edits to reach ECS copies without rebinding individual instances. The
renderer includes effective quality in batching and uniform dirtiness, preserving
unchanged-frame skips. Quality edits require no additional GPU buffer or texture;
the prepared view table remains immutable. The sample's controls queue
validated input and apply it on its existing App update callback.

This is an approximate image-based far LOD. Even two parallax corrections cannot
recover hidden surfaces. Close silhouettes and disocclusions can distort, and
view blends soften material detail. It does not match the layered tracer's
geometric fidelity. Inputs need not be closed solids for octahedral mode; the
static material limitations below still apply. Choose LOD distances from visual
review and measurements in the target scene.

## Layered representation

Layered mode rasterizes every front and back intersection along the unsigned X,
Y and Z axes. At each pixel it sorts the intersections and uses oriented winding
to form the union of closed solids. Overlapping solids remain occupied instead
of creating the false holes produced by parity/XOR. External interval boundaries
retain sampled albedo and outward shading normals, including back boundaries.
Coplanar ties consistently select the later prepared triangle/material.

The `layered` reference profile uses 128×128 axis grids, at most eight occupied
intervals per ray, and four-pixel tile padding. Two 1088×1288 RGBA8 atlases store:

- sRGB surface color with valid-slot coverage;
- numeric linear octahedral normals and 16-bit depth endpoints;
- front/back interval pairs and a conservative quadtree of depth bounds/counts.

The pair occupies exactly **11,210,752 texel bytes (10.69 MiB)** at this setting.
Container metadata, retained CPU data and GPU allocation overhead are additional.
Layered data has **one stored texture level**. Interval endpoints, counts and
hierarchy bounds are numeric fields; ordinary image-mip averaging would corrupt
the representation. Empty surface slots remain empty, without depth dilation.

The version 4 `.kimp` container contains four vertices, six indices, bounded
metadata, a prepacked 3,168-byte GPU descriptor and two prepared KTX2 payloads.
Loading admits the layout and copies the prepared resources. It performs no
model conversion, capture rendering, atlas rebuilding, hierarchy generation or
view-table packing.

## Layered rendering and work limits

The existing billboard vertex path conservatively covers the object's projected
sphere. A fragment chooses a dominant ray axis, traverses the prepared hierarchy,
and tests the occupied interval prisms in candidate cells. It resolves a surface
hit before selecting nearby boundary color/normal attributes from the axis data.
That hit drives fragment depth and the existing lighting path. This mode uses no
ordered coverage dithering.

Tracing has variable work. The prototype caps traversal at 256 charged node visits
and bounds interval tests by `MaxIntervals`. It traverses the hierarchy without
per-fragment stacks: empty regions advance to exact XY cell exits, and changed
cell bits select the next subtree. Budgeted incident-cell queries retain closed
corner contacts and thin layers without epsilon stepping. Before descending, a
node's conservative depth bounds also skip the preceding empty ray segment;
exact cell contacts retain the original traversal. Attribute selection
also reads the axis datasets. **This is not a six-read shader**, and a single
quad does not establish a performance win. Traversal exhaustion and invalid
inside-volume viewing conditions produce a magenta diagnostic rather than an
accepted empty result.

Camera or object movement changes ordinary transforms and GPU ray evaluation.
It does not trigger CPU model scans, geometry generation, atlas processing,
readback or geometry or texture reuploads. Geometry, textures and the static
descriptor remain resident.

## Supported scope and bounds

Inputs use the static, flattened multi-primitive representation produced by
[model/build.GLB](../model/README.md). Layered geometry must represent consistently
oriented, closed solids. Open rays, substantial negative winding and actual
interval-budget overflow fail the bake. The baker does not silently truncate
layers or turn arbitrary open meshes into solids.

A closed negative-winding excursion may be tolerated only within `Radius*1e-4`,
with its count and maximum depth exposed by `BakeWithReport`. The maintained
128px tree has one such self-intersection pocket, approximately 0.0000681 world
units deep. Positive occupied intervals are not removed by this tolerance.
Coincident-event grouping uses the tighter `Radius*1e-6` depth tolerance.

Layered resolution must be a power of two from 32 to 256. `MaxIntervals` ranges
from 1 to 16 and defaults to 8 in layered mode. Atlas dimensions must fit 4096 pixels
per axis; some resolution/padding/interval combinations therefore fail even
within those individual ranges. Offline all-intersection storage is capped at
1,048,576 retained fragments per axis and fails explicitly on overflow.

Materials may have distinct opaque color/albedo and normal maps, but must share
one roughness. Mapped AO and unsupported material features fail explicitly.
Animation, transparency and metallic workflows remain outside this static model
profile. Instance roots require finite, nonsingular, positive-determinant
transforms. Unload rejects live instances; App close releases owned resources.
The shader requires a perspective camera outside the baked bounding sphere.

Finite grid resolution can cause stepped silhouettes, missed thin features and
attribute changes at axis handovers. The numeric prototype has no filtered mip
hierarchy or automatic LOD switching. It has no animation baking or cast-shadow
capture; prefabs must first be flattened by their build processor.

## Measurements

GPU duration and device FPS require separate measurements from the 6/9/12
sampling instruction counts above. The
[comparison sample](../../samples/impostors/README.md#representation-limits)
explains how to inspect approximation errors.

## Explicit capture baselines and compatibility

`Mode: impostorbuild.Octahedral` defaults to an 8×8 grid with 80px captures,
padding 4 and three mips. Its two BC1 atlases have 704×704, 352×352 and
176×176 levels, totaling 650,496 block payload bytes (0.62 MiB), before metadata.
The comparison sample temporarily chooses the 7×7/256px layout above.

`Mode: impostorbuild.Angular` retains the azimuth/elevation baseline. It uses
four views/eight texture reads and the configured elevation range; its defaults
are 128px, eight azimuths and three rows spanning −30 to +30 degrees. Existing
angular-specific options also select this mode for recipe compatibility.

Format v4 uses `KIMPOST4` and resource schema `karty.impostor@4`, capped at 192 MiB.
It carries explicit storage metadata: BC1, ETC2 RGBA or ASTC6x6 for published Angular/Octahedral captures,
exact RGBA8 for the layered reference. The 3,168-byte prepared uniform adds a
storage vector before the capture table. Runtime admission checks storage,
container profiles, color space, dimensions and mip counts before App textures
are created. CPU loading remains valid before `Run`; the active backend rejects
unsupported compression at GPU preparation.

Versions 1–3 are rejected and must be rebuilt. The action revision
`native-view-atlases@10` invalidates affected impostor outputs while unchanged upstream
imports can be reused. Format v4 is the current wire format.

The sample now publishes separate BC1 desktop and ASTC6x6 mobile files, retaining
256 px captures on both platforms. Mobile padding is 16 px for block alignment;
desktop padding remains 8 px. It reads only the supported profile asynchronously
at startup/model selection, using generated path/size/SHA-256 descriptors. Camera
and UI updates continue while a file is pending. Mobile atlases preserve RGB
normals and depth alpha and use the existing non-BC1 shader path. No runtime
transcoding or normal-component reconstruction occurs. See the
[build options](build/README.md).
