# Generate image maps with Materialize

[Asset pipeline](../README.md) · [Generators](README.md)

Use `assetbuild.Materialize(options)` when you have a color texture but no
matching surface-detail maps. It generates a normal map, which changes how light
responds to small surface details, and an ambient-occlusion map, which darkens
small creases. These are estimates derived from the picture; authored detail
maps remain useful when you need a specific surface shape.

The plugin uses the Materialize 2.0.0 CLI from
[AiGameKit's Materialize tool](https://github.com/maikramer/AiGameKit/tree/1a3fe7d052a464d3217ef66d2bce9889d3946792/Materialize).
It keeps the original color image and adds generated maps for later processing
and publication. The source artwork is unchanged.

Materialize needs a supported GPU on the build machine. Ashura selects its
pinned native package through mise unless you supply an executable. The game
does not run this tool.

## Settings and defaults

`assetbuild.MaterializeOptions`:

| Setting | Default | Meaning |
| --- | --- | --- |
| `Executable` | Empty | Use tracked `MATERIALIZE_CLI` when present, otherwise the managed pinned 2.0.0 package. Explicit values override both. |
| `Include` | Empty | Select every albedo resource; otherwise match extensionless resource identities. |
| `Preset` | `"default"` | Preset forwarded to the CLI. |
| `Maps` | `[]string{"normal", "ao"}` | Generate distinct `normal` and/or `ao` maps. Empty selects both. |
| `NormalStrength` | `nil` | Use the selected CLI preset; a pointer overrides normal strength. |
| `HeightContrast` | `nil` | Use the selected CLI preset; a pointer overrides height contrast. |
| `AODepthScale` | `nil` | Use the selected CLI preset; a pointer overrides AO depth scale. |
| `Seamless` | `false` | Explicitly pass `--no-seamless`; true passes `--seamless`. |
| `Environment` | Empty | Override the tracked environment passed to the tool. |
| `BackendIdentity` | Empty | Disable action caching. A supplied identity must identify the actual GPU/driver/backend to enable reuse safely. |
| `AllowSoftware` | `false` | Reject software adapters; true allows them for offline preparation. |
| `Timeout` | `0`: 2 minutes | Deadline for each tool invocation. Negative values are invalid. |

Numeric overrides are `*float64` pointers whose values must be finite,
nonnegative and at most `math.MaxFloat32`; `nil` differs from a pointer to zero.
The plugin leaves preset-specific numeric
defaults to Materialize rather than duplicating them. Inputs must be standalone
albedo images, with no packed regions. Generated map identities must not collide
with authored or already generated resources.

## How to use it

This pipeline imports only albedo sources, so default normal/AO generation cannot
collide with authored normal/AO files:

```go
assetbuild.Pipeline{
    Name: "surfaces", Input: []string{"assets/*-albedo.png"},
    Import: []assetbuild.Plugin{assetbuild.Images()},
    Process: []assetbuild.Plugin{
        assetbuild.Resize(assetbuild.ResizeOptions{MaxDimension: 256}),
        assetbuild.Materialize(assetbuild.MaterializeOptions{Seamless: true}),
        assetbuild.Mipmaps(),
    },
    Output: []assetbuild.Plugin{assetbuild.KTX2(), assetbuild.GoCatalog()},
}
```

This example deliberately leaves `BackendIdentity` empty, so generation executes
on each build. Set it to a verified backend identity when enabling cached reuse;
do not use a generic constant that conceals driver changes. Use `Maps:
[]string{"normal"}` when keeping an authored AO map. Add [SSBump](image-ssbump.md)
after Materialize to derive RNM maps, and before [Mipmaps](../processors/image-mipmaps.md) or
[Atlas](../processors/image-atlas.md).

[Complete recipe](../getting-started.md) · [Source and API](../../../pkg/assetbuild/materialize.go) · [Pinned tool resolver](../../../pkg/assetbuild/materialize_tools.go)
