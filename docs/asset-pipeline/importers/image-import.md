# Import images

[Asset pipeline](../README.md) · [Importers](README.md) · Images

Start with `assetbuild.Images()` for textures, UI artwork and sprites. It decodes
PNG and JPEG files during the build so later plugins can resize, pack or encode
them. It reads `Pipeline.Input` and ignores other file extensions.

The importer does not resize, generate mipmaps, compress or write output files.
Choose those steps explicitly after import.

## Settings and defaults

There are no plugin options. Put source paths or patterns in `Pipeline.Input`.
Each source must be at most 32 MiB encoded and 4096 pixels in either dimension.

The path without its extension, relative to the input pattern's fixed directory,
becomes the name used by later plugins. With `assets/**`, for example,
`assets/ui/play.png` becomes `ui/play`. Generated Go names include that path;
conflicting names fail the build.

Filename endings tell the pipeline what the pixels mean. Matching is
case-insensitive:

| Filename ending, before extension | Role | How pixels are treated |
| --- | --- | --- |
| `-normal`, `_normal`, or `normal` | `UsageNormal` | Linear tangent-space normals; filtering renormalizes vectors. |
| `-height`, `_height`, or `height` | `UsageHeight` | Linear grayscale relief for SSBump generation. |
| `-ao`, `_ao`, or `ao` | `UsageAO` | Linear ambient occlusion values. |
| `-ssbump`, `_ssbump`, or `ssbump` | `UsageSSBump` | Linear directional lighting coefficients. |
| `-reflection`, `_reflection`, or `reflection` | `UsageReflection` | An sRGB panorama for reflection preparation. |
| Anything else | `UsageAlbedo` | sRGB color. |

## How to use it

This [recipe fragment](../getting-started.md) caps texture size, creates mipmaps
and writes embedded KTX2 assets:

```go
assetbuild.Pipeline{
    Name: "textures", Input: []string{"assets/**"},
    Import: []assetbuild.Plugin{assetbuild.Images()},
    Process: []assetbuild.Plugin{
        assetbuild.Resize(assetbuild.ResizeOptions{MaxDimension: 1024}),
        assetbuild.Mipmaps(),
    },
    Output: []assetbuild.Plugin{assetbuild.KTX2(), assetbuild.GoCatalog()},
}
```

For height images, add [SSBump](../generators/image-ssbump.md) before mipmaps and KTX2.
For panoramas, use [ReflectionProbes](../processors/image-reflection-probes.md) to
prepare the cube faces. UI artwork and sprites have their own publishers:
[UI images](../publishers/ui-images.md) and
[2D sprite images](../publishers/sprite-images.md).

[Go implementation](../../../pkg/assetbuild/images.go) ·
[Image processing guide](../../../pkg/assetbuild/README.md)
