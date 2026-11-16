# Publish sprite images

[Asset pipeline](../README.md) · [Publishers](README.md)

`spritebuild.Images()` publishes a typed catalog of `*draw2d.Image` handles and
GPU-ready pages. Use it for sprites, with optional shared atlas pages.
The generated catalog embeds those pages. Choose RGBA8 for broad device
support, BC1 for fully opaque pages, or BC7 for compressed pages with alpha.
Offline compression uses [Basis Universal](https://github.com/BinomialLLC/basis_universal).

Import `spritebuild "github.com/karty-game/karty-webgpu/pkg/draw2d/build"` and,
when selecting formats, `"github.com/karty-game/karty-webgpu/pkg/texture"`.

## Settings and defaults

Pass no arguments or one `spritebuild.ImagesOptions` record.

| Setting | Default | Meaning |
| --- | --- | --- |
| `OpaqueFormat` | `texture.RGBA8` | Pages whose alpha is entirely 255. Accepts RGBA8, BC1 or BC7. |
| `AlphaFormat` | `texture.RGBA8` | Pages containing any translucent texel. Accepts RGBA8 or BC7. |
| `Compression.LDRQuality` | `nil`: BC1 effort 0, BC7 effort 2 | Optional pointer to encoder effort 0–4. |
| `Compression.Timeout` | `0`: 2 minutes | Timeout for one compressed page. |

`Compression` is `assetbuild.BlockCompressionOptions`; its SSBump/RGBE/HDR
settings do not affect these color pages. Format selection happens per page,
so a transparent sprite on an atlas selects `AlphaFormat` for the whole page.

## How to use it

Put this pipeline in the [recipe wrapper](../getting-started.md#prepare-some-textures):

```go
{
    Name: "sprites", Directory: "sprites", Input: []string{"assets/sprites/**"},
    Import: []assetbuild.Plugin{assetbuild.Images()},
    Output: []assetbuild.Plugin{spritebuild.Images(spritebuild.ImagesOptions{
        OpaqueFormat: texture.BC1,
        AlphaFormat: texture.BC7,
    })},
},
```

Use `spritebuild.Images()` for uncompressed portable RGBA8 output. Compressed
pages require device BC support and dimensions divisible by four; invalid
dimensions fail rather than resizing the artwork. No runtime format fallback
is performed. Add [Atlas](../processors/image-atlas.md) to `Process` for shared pages, optionally
preceded by [Resize](../processors/image-resize.md). Sources must be albedo with exactly one level;
omit `Mipmaps`, `KTX2` and `GoCatalog`.

The output contains `assets.go`, `pages/*.{rgba,bc1,bc7}` as selected, and
`atlas.json` for packed regions. With `Config.Output: "generated"`, import
your project's `generated/sprites` package and call `Load()` once per asset
lifetime. Use a generated field with `app.Screen2D().NewSprite(images.Icon)`;
field names depend on source names. The sprite starts at its natural dimensions.

See the [2D image guide](../../../pkg/draw2d/build/README.md) for full runtime usage.
