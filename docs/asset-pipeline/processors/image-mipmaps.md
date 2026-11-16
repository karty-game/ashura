# Generate image mipmaps

[Asset pipeline](../README.md) · [Processors](README.md)

Use `assetbuild.Mipmaps()` to prepare smaller copies of a texture for objects
seen at a distance. The GPU can choose a size close to the pixels it draws,
reducing shimmering and unnecessary texture reads.

Filtering depends on the image's purpose: color maps are averaged in linear
light, normal maps keep valid surface directions, and height/AO/lighting values
are averaged as linear numbers. These levels are built ahead of time, so the
game does not generate them.

## Settings and defaults

There are no plugin options. Standalone images receive a full chain down to
1×1. An image producer may set `Image.MaxMipLevels`; zero permits a full chain.
[Atlas](image-atlas.md) sets a positive safe cap, which this plugin preserves. An atlas
with the default one-level cap stays base-only.

## How to use it

Run mip generation after resizing, SSBump generation and atlas packing, before
texture encoding:

```go
Import: []assetbuild.Plugin{assetbuild.Images()},
Process: []assetbuild.Plugin{
    assetbuild.Resize(assetbuild.ResizeOptions{MaxDimension: 512}),
    assetbuild.Mipmaps(),
},
Output: []assetbuild.Plugin{assetbuild.KTX2(), assetbuild.GoCatalog()},
```

For an atlas with three levels, set `AtlasOptions{..., MipLevels: 3}` before
this plugin. The atlas padding requirements still apply.

[ReflectionProbes](image-reflection-probes.md) already creates face-safe mips; send
its output directly to KTX2 rather than regenerating it with this general image
filter. Lightmap publishers similarly own their chart-safe mip generation.

[Complete recipe](../getting-started.md) · [Source and API](../../../pkg/assetbuild/mipmaps.go)
