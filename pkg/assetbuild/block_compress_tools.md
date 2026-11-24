# Offline GPU texture compression

`CompressRGBERNM`, SSBump, opaque LDR and ETC1S paths call embedded
Basis Universal 2.50 libraries through Go/wazero. No KTX CLI,
Basis CLI, Node runtime, source compiler, download or separate Karty tool release
is required. These functions run only during offline asset preparation; games
receive prepared GPU formats.

The [embedded library provenance](../../internal/basiscodec/third_party/README.md)
records exact source paths, commit, checksums and notices. The C-API modules are
unchanged upstream artifacts. A build lazily owns one serialized encoder and
transcoder session and reuses their initialized instances across asset actions.
Cache-only builds initialize neither. Standalone function calls create temporary
sessions.

Large native ASTC6x6 exports process block-row strips with two bounded offline
workers, reusing the build session and temporarily owning one additional encoder.
They share the original two-minute operation deadline, preserve authored mip
pixels and clamp partial block edges. The extra instance closes after the workers
join. Each module retains its existing 1 GiB memory ceiling; this is not a runtime
pool or an application-side transformation.

The embedded endpoint encoders are single-threaded. BC1 punchthrough masks are
applied offline in Go after endpoint encoding; no replacement native encoder is
needed. No executable, component or thread-selection options remain.

RNM conversion decodes Karty's RGBE convention (`RGB / 255 * 2^(A-128)`) into
float32 input, encodes HDR UASTC at `HDREffort` 0..4, and exports BC6H blocks.
Existing finite half-float-domain checks and the one-authored-level constraint
remain. This preserves normalized radiance and the page scale. It does not use
Radiance `.hdr`, whose `/256` convention differs. The intermediate HDR UASTC is
discarded; browsers receive native BC6H KTX2. Output admission does not establish
an all-content visual-error bound; lower effort still needs content qualification.

SSBump preparation encodes the authored linear RGBA8 coefficient mip chain as
UASTC, then exports BC7 or ASTC4x4. These independent numeric coefficients are not
tangent normal vectors: no normal-map transform, resizing or mip generation is
applied. `PrepareSSBumpUASTC` returns a validated intermediate;
`SSBumpQuality: nil` preserves quality 4 and an explicit pointer selects effort
0..4. The Sponza recipe explicitly chooses effort 0 for desktop SSBump. The
planner can cache the intermediate once for multiple exports. Its identity
includes source and relevant preparation options; explicit KTX override content
and components are dependencies only when that override is selected. HDR effort
and mobile RGBE quality do not affect SSBump preparation.

`RGBEQuality` independently selects 0..4 UASTC effort for mobile RGBE mantissas.
Nil preserves quality 3. EAC exponent bytes remain exact. Lower effort changes
approximation error while GPU formats, dimensions, authored mips and payload
sizes remain unchanged. Recipes must select effort explicitly after measuring
both encoding latency and decoded numeric error.

`TranscodeSSBumpNative` accepts the UASTC intermediate and a BC7 or ASTC4x4 target.
It validates container bounds, numeric color space, block layout and mip ranges,
then validates exported dimensions and mip count and writes canonical KTX2.
UASTC is only a cached build artifact; browsers receive native blocks without a
runtime transcode.

Each separate stage has a bounded deadline. `CompressSSBumpBC7` and
`CompressSSBumpASTC` share one total `Timeout` across preparation and transcoding
(two minutes when zero). Cancellation terminates guest execution and publishes
no partial output. Native GPU output containers have one format and no
supercompression or extra metadata.

BC6H and BC7 require the device's BC texture compression feature. Selecting a
different device format is a build/publishing policy; these functions do not
silently produce an uncompressed fallback.

## BC1 color, coverage and numeric atlases

`CompressBC1` accepts prepared linear or sRGB RGBA8 KTX2 with four-texel-aligned
base dimensions. It retains every authored mip, including smaller sub-block
tails. `LDRQuality` selects 0..4 and defaults to 0; one total `Timeout` bounds
encoding, transcoding and the mask pass. Source bytes remain immutable.

The embedded Basis transcoder supports opaque BC1, so Go applies punchthrough
coverage to its encoded endpoints offline. Source alpha at least 128 becomes
opaque; lower alpha becomes transparent. Mixed blocks use three opaque palette
colors and a transparent selector. Opaque RGB blocks whose black selector would
otherwise become transparent under WebGPU's BC1 RGBA interpretation are repaired
too. This deliberately trades continuous alpha and some color precision for
eight-byte blocks; numeric values must live in RGB rather than alpha.

Output always uses Vulkan 133 (linear) or 134 (sRGB), with the strict BC1
alpha-present data descriptor. GPU upload uses the existing BC feature and
BC1 RGBA format directly. No conversion runs during loading or rendering;
devices without BC support need a separate prepared format. RGB-only Vulkan
131/132 containers remain supported by the existing opaque LDR path.

## LDR lightmap families and shared native exports

`PrepareLDRLightmapCoefficients` accepts one RGB or three RNM linear RGBA8 KTX2
sources, with matching dimensions and one authored mip each. It decodes Karty's
existing `RGB / 255 * 2^(A-128)` convention and finds the actual maximum across
the complete family. Normalized RGB8 values share the returned physical scale;
there is no exposure clipping, but ordinary 8-bit quantization remains. An
all-black family retains its incoming scale. Invalid or unrepresentable physical
scales fail admission. The requested Linear or SRGB transfer is encoded offline
and tagged in KTX metadata; alpha becomes 255. Source bytes remain immutable.
A 65,536-byte exponent/mantissa output lookup avoids per-pixel powers/transfers.

`PrepareOpaqueLDRUASTC` validates opaque RGBA8 with four-texel-aligned base
dimensions, retaining authored smaller mip extents, and
prepares one shared UASTC intermediate. `LDRQuality` selects 0..4 and defaults
to 0 independently of the existing SSBump and mobile RGBE defaults. The same
embedded codec session and total deadline rules apply.

`TranscodeLDRNative` accepts that LDR UASTC, numeric SSBump UASTC (including
unrelated alpha), or the existing bounded opaque SRGB ETC1S albedo profile. It
exports native BC1, BC7 or ETC2RGB while retaining transfer and authored mip
extents. RGB targets omit unused numeric alpha; opaque color preparation rejects
transparency before encoding. No decoder or additional texture transformation
runs during game loading or rendering. Linear/SRGB Vulkan formats are respectively 131/132 for BC1,
145/146 for BC7, and 147/148 for ETC2RGB.

New LDR native containers serialize mip payloads smallest-first, matching the
KTX2 container contract and external KTX validation. Existing legacy native
export functions retain their container layout. New-profile lightmaps,
SSBump and albedo should all use `TranscodeLDRNative` for canonical containers.
`TranscodeSSBumpNative` additionally admits ETC2RGB for shared-preparation exports;
target admission does not establish numeric SSBump quality. The existing
ETC1-subset transcode remains unqualified for the new mobile SSBump default.
