# Publish KTX2 images

[Asset pipeline](../README.md) · [Publishers](README.md)

`assetbuild.KTX2()` publishes GPU-ready texture files from image resources.
The default writes uncompressed RGBA8 KTX2. Explicit options select native BC1
or BC7 blocks, encoded offline through the embedded WASM codec. KTX2 is a
container; choosing it alone does not enable compression.

Choose RGBA8 for broad device support, BC1 for opaque colors or cutout alpha,
and BC7 when smooth alpha or better quality matters. The [KTX format](https://www.khronos.org/ktx/)
stores the resulting texture. Compression uses [Basis Universal](https://github.com/BinomialLLC/basis_universal)
through Go-hosted WASM during the build.

## Settings and defaults

Pass no options or one `assetbuild.KTX2Options` value:

| Setting | Default | Meaning |
| --- | --- | --- |
| `Format` | `texture.RGBA8` | Common storage format: RGBA8, BC1 or BC7. |
| `UsageFormats` | Empty | Per-role overrides of `Format`, keyed by constants such as `assetbuild.UsageAlbedo`. |
| `Compression` | Zero struct | Offline `BlockCompressionOptions`, described below. |

The `Compression` fields are shared with other offline exporters:

| Field | Default | Effect in this plugin |
| --- | --- | --- |
| `LDRQuality` | `nil`: BC1 effort 0, BC7 effort 2 | Pointer selecting encoder effort 0..4. An explicit pointer to 0 selects fastest effort. |
| `Timeout` | `0`: 2 minutes | Total encoding deadline for each compressed texture. Negative values are invalid. |
| `SSBumpQuality` | `nil` | Unused here; KTX2's BC1/BC7 path uses `LDRQuality` for every role. |
| `RGBEQuality` | `nil` | Unused here. |
| `HDREffort` | `0` | Unused here. |

BC1 uses one-bit alpha, with values below 128 transparent; BC7 retains smooth
alpha. BC1 uses 8 bytes per 4×4 block and BC7 uses 16, versus 64 bytes of RGBA8
for those texels, before container overhead. Base dimensions must be multiples
of four for either compressed format. The target device must support BC
compression; this plugin supplies no automatic fallback or runtime transcoder.
ETC2, ASTC and BC6H are not options for this image exporter.

## How to use it

Import `texture` from `github.com/karty-game/karty-webgpu/pkg/texture`, then use:

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

[Complete recipe](../getting-started.md) · [Source and API](../../../pkg/assetbuild/ktx2.go) · [Compression API](../../../pkg/assetbuild/block_compress.go)
