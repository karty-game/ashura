# Publish UI images

[Asset pipeline](../README.md) · [Publishers](README.md)

`uibuild.Images()` publishes a typed catalog of UI images and their GPU-ready
RGBA8 pages. Use it for panel backgrounds, icons and nine-slice artwork.
The catalog embeds the prepared pages, so game code can load them without
reading PNG or JPEG files.

Import `uibuild "github.com/karty-game/ashura/pkg/ui/build"`.

## Settings and defaults

This plugin has no options. It consumes albedo images with exactly one RGBA8
level. Source dimensions are preserved unless an earlier `Resize` changes them.
The output is `assets.go`, `pages/*.rgba` and, for packed images, `atlas.json`.
Named catalog fields retain the source image names even when pages are packed.

## Pack a UI atlas

Use `uibuild.Atlas` as an entry in `Config.Pipelines` in the
[recipe wrapper](../getting-started.md#add-a-recipe).

```go
uibuild.Atlas(uibuild.AtlasOptions{
    Name: "ui", Include: "ui/**", MaxSize: 512, Padding: 2,
}),
```

The helper imports images under `assets/`, packs the images matching `Include`
into a clamped atlas, and publishes the UI catalog. `Include` matches
extensionless names relative to `assets/`. Other images get standalone pages.
`Name` identifies the pipeline and atlas. `MaxSize` and `Padding` use the
[atlas defaults](../processors/image-atlas.md) when zero.

With `Config.Output: "generated/assets"`, import your project's
`generated/assets` package, then call `Load()` once and share the catalog.
Each call owns its pages, and regions within one catalog share their page. Use the
generated image fields in `ui.PanelBackground.Image`, for example. Nine-slice
cuts use the image dimensions after any resizing.

For custom source roots, resizing, or standalone pages, construct a pipeline
using `assetbuild.Images()` and finish with `uibuild.Images()`.

See [Atlas](../processors/image-atlas.md), [Resize](../processors/image-resize.md) and the
[UI asset guide](../../../pkg/ui/build/README.md) for runtime examples.
