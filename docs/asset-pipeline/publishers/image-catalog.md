# Publish an image catalog

[Asset pipeline](../README.md) · [Publishers](README.md)

`assetbuild.GoCatalog()` generates the `assets` Go package for prepared image
resources. Each named accessor returns a typed `ktx2.Asset` backed by an embedded
KTX2 file, so importing the generated package makes those payloads part of the
application binary.

Use it when code should reference textures by name without managing separate
file requests. For UI artwork or sprites, use their dedicated image publishers
instead; they return the image handles those renderers need.

## Settings and defaults

There are no plugin options. The plugin writes `assets.go` beneath the pipeline's
publication directory, with package name `assets`. It must follow
[KTX2](image-ktx2.md), which publishes the referenced `.ktx2` files.

For [atlases](../processors/image-atlas.md), it also writes `atlas.json`, page accessors and
`<SourceName>Region()` accessors containing page assets, pixel bounds and UVs.
Use those region accessors to draw individual source images from a shared page.
Conflicting generated names fail the build.

## How to use it

```go
assetbuild.Pipeline{
    Name: "textures", Input: []string{"assets/**"},
    Import: []assetbuild.Plugin{assetbuild.Images()},
    Process: []assetbuild.Plugin{assetbuild.Mipmaps()},
    Output: []assetbuild.Plugin{assetbuild.KTX2(), assetbuild.GoCatalog()},
}
```

With `Config.Output: "generated/assets"`, import your project's
`generated/assets` package. A source named `brick-albedo.png` produces an
`assets.BrickAlbedo()` accessor, which returns an albedo `ktx2.Asset`. The
accessor's name follows the source-relative logical identity, including
subdirectories.

Use separate `Pipeline.Directory` values when two pipelines emit `assets.go`;
pipeline names alone do not create subdirectories. UI and sprites have their
own exporters: [UI Images](ui-images.md) and [draw2d Images](sprite-images.md).

[Complete recipe](../getting-started.md) · [Source](../../../pkg/assetbuild/catalog.go) · [Generated declaration rules](../../../pkg/assetbuild/catalog_code.go)
