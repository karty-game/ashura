# Resize images

[Asset pipeline](../README.md) · [Processors](README.md)

Use `assetbuild.Resize(options)` to keep source textures from making your build
larger than it needs to be. `MaxDimension` is a useful default choice: it limits
the longest side, preserves the image's shape and leaves smaller images alone.
Use explicit width and height when you need an exact output size.

Color is resized in linear light to avoid dark edges, while normal maps keep
valid surface directions. Shrinking averages the covered source pixels;
enlarging uses bilinear interpolation.

## Settings and defaults

`assetbuild.ResizeOptions`:

| Setting | Default | Meaning |
| --- | --- | --- |
| `Width` | `0` | Target width. With only width set, preserve aspect ratio. |
| `Height` | `0` | Target height. With only height set, preserve aspect ratio. |
| `MaxDimension` | `0` | Bound the larger dimension, preserve aspect ratio and never upscale. |
| `Include` | Empty | Select all image resources; otherwise match extensionless logical identities. |

Choose `Width`/`Height` or `MaxDimension`, never both modes. All dimensions must
be between 0 and 4096, with at least one positive value. Setting both width and
height requests that exact size, even if the aspect ratio changes. Explicit
width/height may upscale. A zero options struct is an error.

## How to use it

Put resize after [Images](../importers/image-import.md), before [Atlas](image-atlas.md) and
[Mipmaps](image-mipmaps.md). For input `assets/**`, this selects `assets/ui/...`:

```go
Process: []assetbuild.Plugin{
    assetbuild.Resize(assetbuild.ResizeOptions{
        MaxDimension: 512,
        Include: []string{"ui/**"},
    }),
    assetbuild.Mipmaps(),
},
```

For an exact square texture, use `ResizeOptions{Width: 256, Height: 256}`.
Packed atlases cannot be resized because their region mappings would be invalid.
The direct `ResizeImage` helper serves other offline importers; it requires one
base level and an empty `Include` list.

[Complete recipe](../getting-started.md) · [Source and API](../../../pkg/assetbuild/resize.go)
