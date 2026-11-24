# Offline lightmap baking

`github.com/karty-game/ashura/pkg/lightmap/bake` is a host-only build API.
It prepares physical RGB diffuse lighting and calibrated RNM coefficients from
one transport solver. The game receives completed textures; this package adds no
runtime lightmapper, fitting solver, image decoder or worker to an engine build.

This guide describes the production units, projection and implemented APIs.
Phone qualification and game frame pacing require separate device checks.

## Fixed-count hardware bake

Prepare `Scene` with a nonempty ID/space, positive `MetresPerUnit`, triangles,
linear reflectance textures and declared emitters. `SceneFromModel` converts an
admitted prepared model offline. Point/spot sources use radiant intensity;
area/environment sources use radiance. `LegacyRangeV1` is an explicit adapter
for the previous finite-range source function.

The following function assumes UV1 triangle ordering matches `scene.Triangles`.
The memory ceiling comes from build configuration. The executor uses the same
wgpu-native library setup as native rendering.
Sample count and bounce depth are example quality choices, not device budgets.

```go
import (
    "context"
    bake "github.com/karty-game/ashura/pkg/lightmap/bake"
)

func gather(ctx context.Context, scene bake.Scene, uv1 [][3]bake.Vec2,
    width, height uint32, memoryBytes uint64,
) (bake.Result, error) {
    prepared, err := bake.PrepareScene(scene)
    if err != nil { return bake.Result{}, err }
    receivers, err := bake.PrepareReceivers(scene, uv1, width, height)
    if err != nil { return bake.Result{}, err }
    worker, err := bake.NewWorker(ctx, bake.WorkerOptions{
        MemoryBytes: memoryBytes,
    })
    if err != nil { return bake.Result{}, err }
    defer worker.Close()
    return worker.Bake(ctx, prepared, receivers, bake.Options{
        Outputs: bake.OutputRGB | bake.OutputRNM,
        Samples: 256, Bounces: 4, Seed: 29,
    })
}
```

`PrepareReceivers` clips actual covered UV1 support, retains slivers and samples
uniformly in covered texel area. Positive-area UV overlaps are rejected. Its
materialized world patches may use a different triangulation from the complete
visibility scene. `RemapReceiverMaterials` binds page-model material identity to
that scene; the maintained level plugin performs this step. Front/back lighting
requires independent receiver geometry/charts; sharing one fabric map is not an
automatic fallback.

Keep the worker and prepared scene for subsequent pages. `Bake` tiles receiver
work to admitted device/memory bounds while preserving receiver sample identity.
The complete visibility scene and material set must currently fit resident
bounds. The default worker requires an actual admitted hardware adapter and
reports failure instead of silently switching to software. `WorkerOptions.SoftwareForCI`
is an explicit build-only exception: it requests a native Vulkan CPU adapter for
hosted functional CI and records `Device.SoftwareForCI` with the actual adapter.
It is not a local graphics-review path or GPU performance evidence. Adapter
admission checks this explicit selection before creating the device.

The worker is an in-process Go executor using the existing rendering binding.
Our Go/WGSL implementation remains in Ashura; no C helper, subprocess protocol or
separate worker installation is involved. The
[tool requirements](../../../docs/asset-pipeline/tools.md) cover the upstream
`wgpu-native` dependency.

For cacheable preparation, `NativeLibrary` identifies the binding's active library,
`WGPU_NATIVE_PATH`, `./lib` or the local native library. The level asset plugin
installs an official checksum-pinned wgpu-native archive through mise when needed,
then calls `InitializeNativeLibrary` without modifying environment. Direct library
callers can select an installed library explicitly with that function. Native
Go builds use `-tags=nofakecgo`, as the CLI already selects for host recipes. The pipeline fingerprints that
library and the recipe binary/WGSL, plus GPU/driver/backend configuration. Explicit
CI software selection remains separate from hardware evidence.

Cancellation invalidates the executor, stops submission, aborts/drains pending
maps and releases its device resources. Native driver calls already executing
cannot be forcibly killed in-process. Complete memory bounds and result admission
are preserved; no partial result is published.

## Results, reconstruction and projection

`Result.Estimates` contains reference-normal diffuse factor `D = irradiance/pi`,
three RNM fit right-hand-side statistics, diagnostics and resolved RNM maps.
Receiver albedo is excluded and is applied once by the runtime material. RGB
and flat RNM agree before numeric encoding; directional RNM remains a lossy
approximation. It is not a normal-map-capable RGB texture.

`Estimate.Variance` is stable Welford sample spread. `VarianceValid` is false
for one sample. Optional `SecondMoments` diagnostics have their own validity
flag and may be absent when squaring a valid HDR mean would overflow. Quality
operations consume the stable spread, not `E[x*x] - E[x]*E[x]`.

Use `BuildSurfaceEdges` with explicit candidate/edge preparation bounds to derive
reconstruction support from actual patch boundary contact and complete-scene
visibility. Coordinate proximity alone does not connect surfaces. Opposite fabric
sides remain separate. Set `SurfaceEdgeOptions.Support` to the exact consuming
`ReconstructionOptions` so frame/plane/distance-ineligible contacts cannot
contaminate topology admission. Ambiguous contact fails by default.
`BuildSurfaceEdgesReportContext` offers the same graph with cooperative cancellation.
It checks boundary/contact preparation and joins all independent visibility workers
before returning. Worker scheduling changes neither graph order nor filter support.

`BuildSurfaceEdgesReport` additionally accepts explicit `SurfaceIsolateAmbiguity`:
it records every incident receiver and contact, then excludes all links touching
those receivers. Preserve their original estimates and retain the report in the
build artifact; isolation does not discard geometry, coverage or raw transport.
`Worker.Reconstruct` reads finalized immutable neighbors and fits the filtered
statistics on the GPU. Its memory tiling keeps complete halos. Gather all page
receivers for a logical space before calling it; filtering pages independently truncates
seam support. `ReconstructHalo` also accepts explicit bounded cross-tile inputs.

`ReconstructionOptions.MaximumHalo` is an optional scheduling ceiling. Zero uses
the actual packet, negotiated device and worker memory bounds. A positive value
retains an additional explicit receiver ceiling. Both modes preserve each
receiver's complete neighbor support and reconstruction quality.

Reconstruction retains original estimates for zero averaged tangent frames.
Canonical prepared support shares identity indexing with frame partitioning and
constructs CSR links directly; omitted-frame edges still receive complete
admission. Arbitrary-order inputs retain their original validation path.

For generated visible unit-weight support, `PrepareReconstructionSupport` accepts
`ReconstructionAdjacency` rows in canonical original receiver-ID order. Its sealed
result privately owns dense targets and static receiver guides. Reuse it through
`Worker.ReconstructPrepared` with the same ordered receiver identities and guides;
lighting, finalized estimates and filter options may change. Changed geometry or
frames reject before compute. This path keeps unit adjacency compact through halo
selection and produces the same canonical compute packets as weighted support.

The named `CalibrationProfile` uses the actual normalized unoccluded SSBump
angular weights and a nonnegative fit constrained to preserve flat energy.
Visibility-weighted material coefficients are not renormalized. Grazing light and
large perturbed normals can exceed the three-map approximation; output fitting
must report that error rather than adjust exposure.

`EncodeScene`, `EncodeReceiverSet` and `EncodeResult` produce bounded checksummed
build artifacts. Receiver sets use compact typed `KARTYRX2` binary records,
including barycentric support and original frame gradients. After reconstruction,
`EncodeAtlas` writes RGBE atlas data with the supplied chart-owned gutter map;
use the offline native codecs/publication stage to produce BC and ETC2/ASTC
variants. Do not generate ordinary whole-atlas mips across unrelated charts.

## Adaptive sampling and verification

`Worker.BakeBatches` accepts independently identified `BakeBatch` entries sharing
one prepared scene. It prepares every page and memory split before submitting
GPU work, parallelizes independent hashes/packets over available Go workers, and
overlaps one previous output decoder with serial GPU execution. Results retain
input order, per-page options/seeds and complete provenance. Any invalid input or
execution/decoding failure returns no partial result list. Keep input receivers
immutable until return. `Bake` uses the same path for a single entry.

Standard fixed production sampling uses one cacheable logical-space action with
individual page outputs. Adaptive and explicit-resume planning operate per page.

`Worker.BakeAdaptive` accepts `AdaptivePolicy`, optional frozen `AdaptiveState`
and a snapshot callback. It runs an independent pilot, freezes per-receiver
production counts, excludes pilot estimates from published means, and resumes
only committed compatible production work. A frontend must persist snapshots
atomically and restore them on retry; passing nil state/callback restarts an
unfinished page. Strict quality returns `ErrUnconverged` when diagnostic targets
or the requested cap prevent acceptance.

The sampler uses scrambled low-discrepancy sequences. Welford spread divided by
sample count is a useful error diagnostic, **not a calibrated QMC variance or
confidence interval**. Fields named `Confidence` scale a diagnostic tolerance;
values do not imply 95% or any other probability coverage.

`Worker.VerifyRNM(ctx, scene, receivers, estimates, transportOptions, policy)`
performs a separate fixed-count hardware pass. All requested directional
responses use the same traced paths; verification does not run seventeen
independent lighting solvers. It compares full responses with reconstructed
coefficients, retaining uncertainty, probe errors, checked IDs and coverage.
Four fitting means alone cannot establish directional error.

`VerificationPolicy.MaterialDomains` carries actual prepared tangent-normal
samples and source/preparation identities for each material. Composed SSBump
visibility coefficients cannot recover the unoccluded normal domain by
inversion. `RequireMaterialDomain` rejects missing domains;
`RequireAllReceivers` rejects a capped receiver subset. Inspect both coverage and
`MaterialDomainVerified` when interpreting a report. `ErrVerification` carries
a failed representation check; its returned report remains useful diagnostics.

## Maintained level pipeline

Use `level/build.LightmapUVs` with `Production: true`, followed by
`ProductionBake`. Choose RGB/RNM through `PublishOptions.Representation`; both
can reuse one traced result. Final RNM requires explicit directional verification.
`Preview: true` is an explicit development choice and must not be reported as
final quality acceptance. The standalone worker APIs also expose reconstruction,
adaptive state and reports for tools that need explicit orchestration.

The complete acceptance gate includes a new Sponza bake, many-light equal-error
measurements, seam/shadow/bounce review and actual BC plus ETC2/ASTC hardware,
including a physical mobile browser. Existing preview images and BC-disabled
desktop execution do not substitute for these qualifications. Host-side logical-space assembly remains resident, while reconstruction spans
page and runtime chunk boundaries using bounded GPU interiors and immutable
halos as described in the specification.

Explicit conservative support isolation uses `BuildSurfaceEdgesReport` with
`SurfaceEdgeOptions.Ambiguity = SurfaceIsolateAmbiguity`. It reports every
isolated receiver/contact and removes all incident filter links, preserving its
original estimate. The edges-only API rejects that policy so callers cannot
silently discard diagnostics. `Worker.Reconstruct` similarly retains records
whose valid opposing patch frames average to exactly zero tangent: every
incident tangent-weighted kernel contribution is zero. Low-level halo admission
continues to require a unit frame.

`ProductionOptions.ResumeSamples` accepts explicitly declared fixed-count linear
source files. The level plugin checks their checksum, scene and ordered receiver
hashes, exact options/backend, counts and variance. Distinct compatible files
are rejected. These are ordinary tracked build inputs; they do not bypass cache
identity or resource provenance. Adaptive partial progress uses the separate
frozen-state API and cannot be mixed with fixed-count imports.

`Worker.VerifyRNMBatches(ctx, scene, []VerificationBatch{...})` retains one
independent policy, receiver selection, derived random seed and report per input
batch. Compatible reference passes share native GPU dispatch using an explicit
per-receiver seed table. Reports and response values preserve separate
`VerifyRNM` semantics across memory-driven subdivisions; receiver identities are
never rewritten. All batch preparation and split admission finish before the
first dispatch. `ErrVerification` returns completed representation diagnostics;
invalid preparation or execution errors terminate the operation. This is an
offline tool API and adds no game-loading or rendering work.
