# Generate image SSBump maps

[Asset pipeline](../README.md) · [Generators](README.md)

Use `assetbuild.SSBump(options)` to make small bumps and grooves respond to baked
directional lighting. Supply a normal map or grayscale height map; the build
calculates the surface response and how nearby relief blocks incoming light.
The result is a self-shadowed bump map, used with RNM lighting. See the
[lighting glossary](../processors/README.md#terms-used-in-these-guides) for the relationship.

The calculation treats relief as repeating, so it works best with tileable
surface textures. Images used for other purposes remain unchanged.
Derived maps use the `-ssbump`/`_ssbump` suffix in their generated names.
The logical resource identity stays unchanged: converting `stone-height` still
leaves resource ID `stone-height`, while its contained image/source name becomes
`stone-ssbump`. Use the resource ID when binding level material overrides.

## Settings and defaults

`assetbuild.SSBumpOptions`:

| Setting | Default | Meaning |
| --- | --- | --- |
| `Include` | Empty | Select all resources; patterns match extensionless logical identities. Only normal and height images are transformed. |
| `Rays` | `300` | Visibility rays; range 1..4096. |
| `Distance` | `32` | Visibility reach in source texels; range 0.5..256. |
| `Bias` | `0.05` | Positive height offset for visibility origins; at most 32. |
| `KeepNormals` | `false` | Retain normal inputs alongside derived maps. Height inputs are always consumed. |
| `HeightScale` | `8` | Relief, in output texels, for a grayscale height range of 0..1; positive, at most 512. Applies to authored heights. |

Zero numeric values select the listed defaults. Inputs must be unpacked,
base-only, square power-of-two images from 2×2 through 512×512. Total work per
image is bounded by `width × height × Rays × ceil(Distance) <= 2^31`;
512×512 with the default ray settings exceeds that bound. Resize or lower the
ray count/reach for larger maps. More rays and longer reach increase build time.

## How to use it

For `Input: []string{"assets/**"}`, keep authored normals for another material
while deriving SSBump maps:

```go
Import: []assetbuild.Plugin{assetbuild.Images()},
Process: []assetbuild.Plugin{
    assetbuild.Resize(assetbuild.ResizeOptions{
        Width: 256, Height: 256, Include: []string{"*-normal"},
    }),
    assetbuild.SSBump(assetbuild.SSBumpOptions{
        Include: []string{"*-normal"}, KeepNormals: true,
    }),
    assetbuild.Mipmaps(),
},
Output: []assetbuild.Plugin{assetbuild.KTX2(), assetbuild.GoCatalog()},
```

Run SSBump after any resizing and [Materialize](image-materialize.md), before atlas
packing and mip generation. To start from authored relief, use images named
`*-height.png` and select those identities instead. Every height resource sent
to KTX2 must be consumed by this stage. Avoid supplying both a normal and height
map that derive the same output name in one pipeline.

[Complete recipe](../getting-started.md) · [Source and API](../../../pkg/assetbuild/ssbump.go) · [Material sample](../../../samples/standard-materials/asset.config.go)
