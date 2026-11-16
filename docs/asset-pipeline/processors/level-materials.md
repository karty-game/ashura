# Prepare level materials

[Asset pipeline](../README.md) · [Processors](README.md)

Use `levelbuild.Materials` to connect a level's models to the surface-detail maps
you prepared earlier in the recipe. It supplies the directional relief used by
RNM lighting, either from explicit SSBump maps or by deriving it from each
model's normal map. [RNM and SSBump](README.md#terms-used-in-these-guides) describe
how those surface details respond to baked light.

This stage accepts processed images directly. You can resize a height image,
convert it with [SSBump](../generators/image-ssbump.md), then bind it here without first writing a
KTX2 file. [Publish](../publishers/level-publish.md) writes the finished level's textures.

Import `levelbuild` from `github.com/karty-game/karty-webgpu/pkg/level/build`.
Place it in `Pipeline.Process`, after [Scenes](../importers/scene-lightmap-baking.md) and any image
resize, SSBump conversion and mip generation; before
[StaticChunks](level-static-chunks.md).

## Settings and defaults

`Materials(levelbuild.MaterialOptions{...})` takes one setting:

| Setting | Default | Meaning |
| --- | --- | --- |
| `SSBump map[string]map[string]string` | No explicit overrides | Prepared model resource ID → exact original primitive name → processed SSBump image resource ID. |

Without an override, opaque relief is derived offline from the model's normal
map where supported, including its normal strength. The automatic path expects
square power-of-two normals and chooses a mip no larger than 512px. Unknown
primitive names in used models fail instead of silently losing their overrides.
Independent masked objects retain their original materials outside the opaque
bake.

## How to use it

The following fragment uses an imported height image named `stone-height` and a
prepared model resource `assets/building.glb`:

```go
Process: []assetbuild.Plugin{
    assetbuild.Resize(assetbuild.ResizeOptions{
        Include: []string{"*-height"}, MaxDimension: 256,
    }),
    assetbuild.SSBump(assetbuild.SSBumpOptions{
        Include: []string{"*-height"}, HeightScale: 3,
    }),
    assetbuild.Mipmaps(),
    levelbuild.Materials(levelbuild.MaterialOptions{
        SSBump: map[string]map[string]string{
            "assets/building.glb": {"Building/wall/primitive0": "stone-height"},
        },
    }),
    levelbuild.StaticChunks(levelbuild.ChunkOptions{}),
    // ProductionBake follows here.
},
```

Replace the example primitive name with the exact name in your prepared model.
Use `MaterialOptions{}` if automatic relief is sufficient.

[Plugin source](../../../pkg/level/build/materials.go) · [Pipeline guide](../getting-started.md)
