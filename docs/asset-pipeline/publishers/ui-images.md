# Publish UI images

[Asset pipeline](../README.md) · [Publishers](README.md)

`uibuild.Images()` publishes a typed catalog of UI images and their GPU-ready
RGBA8 pages. Use it for panel backgrounds, icons and nine-slice artwork.
The catalog embeds the prepared pages, so game code can load them without
reading PNG or JPEG files.

Import `uibuild "github.com/karty-game/karty-webgpu/pkg/ui/build"`.

## Settings and defaults

This plugin has no options. It consumes albedo images with exactly one RGBA8
level. Source dimensions are preserved unless an earlier `Resize` changes them.
The output is `assets.go`, `pages/*.rgba` and, for packed images, `atlas.json`.
Named catalog fields retain the source image names even when pages are packed.

## How to use it

Put this pipeline in the [recipe wrapper](../getting-started.md#prepare-some-textures):

```go
{
    Name: "ui", Directory: "ui", Input: []string{"assets/ui/**"},
    Import: []assetbuild.Plugin{assetbuild.Images()},
    Process: []assetbuild.Plugin{assetbuild.Atlas(assetbuild.AtlasOptions{
        Name: "ui", Include: []string{"**"}, Clamp: true,
        MaxSize: 1024, Padding: 2,
    })},
    Output: []assetbuild.Plugin{uibuild.Images()},
},
```

Omit `Atlas` for standalone pages. Use a separate pipeline for non-albedo maps.
Do not add `Mipmaps`, `KTX2` or `GoCatalog`: this output plugin owns the page
format and catalog. Atlas packing and source decoding happen during the build.

With `Config.Output: "generated"`, import your project's `generated/ui` package,
then call its `Load()` once and share the returned catalog. Each call creates
independent owned pages; regions within one catalog share their page. Use the
generated image fields in `ui.PanelBackground.Image`, for example. Nine-slice
cuts use the image dimensions after any resizing.

See [Atlas](../processors/image-atlas.md), [Resize](../processors/image-resize.md) and the
[UI asset guide](../../../pkg/ui/build/README.md) for runtime examples.
