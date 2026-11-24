# Publish KTX2 images

[Asset pipeline](../README.md) · [Publishers](README.md)

`assetbuild.KTX2()` publishes GPU-ready texture files from image resources.
The default writes uncompressed RGBA8 KTX2. Explicit options select native BC1
BC7 or ETC2 blocks, plus ASTC4x4 for SSBump coefficients, encoded offline through the embedded WASM codec. KTX2 is a
container; choosing it alone does not enable compression.

Choose RGBA8 for broad device support, BC1 for opaque colors or cutout alpha,
and BC7 when smooth alpha or better quality matters. The [KTX format](https://www.khronos.org/ktx/)
stores the resulting texture. Compression uses [Basis Universal](https://github.com/BinomialLLC/basis_universal)
through Go-hosted WASM during the build.

## Settings and defaults

Pass no options or one `assetbuild.KTX2Options` value:

| Setting | Default | Meaning |
| --- | --- | --- |
| `Format` | `texture.RGBA8` | Common storage format: RGBA8, BC1, BC7, ETC2RGB or ETC2RGBA. |
| `UsageFormats` | Empty | Per-role overrides of `Format`; `UsageSSBump` also accepts ASTC4x4. |
| `Compression` | Zero struct | Offline `BlockCompressionOptions`, described below. |

The `Compression` fields are shared with other offline exporters:

| Field | Default | Effect in this plugin |
| --- | --- | --- |
| `LDRQuality` | `nil`: BC1/ETC2RGB effort 0, BC7/ETC2RGBA effort 2 | Pointer selecting encoder effort 0..4. An explicit pointer to 0 selects fastest effort. |
| `Timeout` | `0`: 2 minutes | Total encoding deadline for each compressed texture. Negative values are invalid. |
| `SSBumpQuality` | `nil`: effort 4 | UASTC effort for ASTC4x4 SSBump coefficients. The Materials sample explicitly uses 2, matching desktop BC7 effort. Other formats use `LDRQuality`. |
| `RGBEQuality` | `nil` | Unused here. |
| `HDREffort` | `0` | Unused here. |

BC1 uses one-bit alpha, with values below 128 transparent; BC7 retains smooth
alpha. BC1 uses 8 bytes per 4×4 block and BC7 uses 16, versus 64 bytes of RGBA8
for those texels, before container overhead. Base dimensions must be multiples
of four for these formats. ETC2RGB uses 8 bytes per 4×4 block; ETC2RGBA and
ASTC4x4 use 16. SSBump ASTC stores independent linear RGB coefficients without
a two-channel normal-map preset or reconstructed Z. The target device must
support the selected compression family. Prepare separate desktop/mobile
variants and select after the device opens. This plugin supplies no automatic
fallback or runtime transcoder. BC6H and ASTC6x6 are not options for this exporter.

## How to use it

Import `texture` from `github.com/karty-game/ashura/pkg/texture`, then use:

```go
Import: []assetbuild.Plugin{assetbuild.Images()},
Process: []assetbuild.Plugin{assetbuild.Mipmaps()},
Output: []assetbuild.Plugin{
    assetbuild.KTX2(assetbuild.KTX2Options{
        UsageFormats: map[string]texture.Format{
            assetbuild.UsageAlbedo: texture.BC1,
            assetbuild.UsageNormal: texture.BC7,
            assetbuild.UsageSSBump: texture.BC7,
        },
    }),
    assetbuild.GoCatalog(),
},
```

Unlisted roles remain RGBA8. Output paths preserve the relative source path with
the extension changed to `.ktx2`. Dimensions and supplied mip levels are retained;
run [Resize](../processors/image-resize.md) and [Mipmaps](../processors/image-mipmaps.md) first when needed. Height sources
must first pass through [SSBump](../generators/image-ssbump.md). Color/reflection textures use sRGB;
normal, AO, SSBump and lightmap textures use linear transfer.

Use [GoCatalog](image-catalog.md) for named embedded asset accessors, or consume the
published KTX2 files separately. [Level publication](level-publish.md) has its own
desktop/mobile texture profiles.

Use `KTX2Variants(primary, mobile)` in one pipeline to share image preparation.
Each argument is a `KTX2Options` value. When the selected formats differ for
an image, the mobile file goes under `mobile/` and `GoCatalog()` adds an
accessor such as `assets.StoneAlbedoMobile()`. Matching formats use the
primary asset and its encoder settings. Downstream image processing and the
atlas region accessors use the primary page. Use the mobile page accessor
with the same UV mapping for a mobile atlas.

[Complete recipe](../getting-started.md) · [Source and API](../../../pkg/assetbuild/ktx2.go) · [Compression API](../../../pkg/assetbuild/block_compress.go)
