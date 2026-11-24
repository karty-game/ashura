# Partition a level into chunks

[Asset pipeline](../README.md) · [Processors](README.md)

Use `levelbuild.StaticChunks` to split a large static scene into grid cells that
can be prepared and loaded as chunks. It clips geometry at cell boundaries and
groups compatible materials within each chunk. Dynamic objects remain separate.

Each logical space is partitioned independently. The lighting bake still sees
all geometry in that space: a distant wall can cast a shadow or bounce light
into another chunk. Splitting the geometry does not split the lighting world.

Import `levelbuild` from `github.com/karty-game/ashura/pkg/level/build`.
Place this plugin in `Pipeline.Process` after [Materials](level-materials.md),
before UV preparation or integrated baking.

## Settings and defaults

`StaticChunks(levelbuild.ChunkOptions{...})` takes:

| Setting | Default | Meaning |
| --- | --- | --- |
| `Size [3]float32` | `[32, 32, 32]` when the entire array is zero | Cell width, height and depth in scene units. All explicit dimensions must be finite and positive. |
| `Origin [3]float32` | `[0, 0, 0]` | Finite grid origin. |
| `MaxChunks uint32` | `4096` | Maximum output chunks. Cannot exceed 4096. |
| `MaxTriangles uint32` | `1 << 20` | Geometry preparation triangle ceiling. Cannot exceed 1,048,576. |

Zero limits select these bounds; they do not remove the limits. Smaller cells
can improve selection granularity while increasing chunk/resource overhead.
This plugin does not perform runtime chunk selection or portal traversal.

## How to use it

```go
levelbuild.StaticChunks(levelbuild.ChunkOptions{
    Size: [3]float32{32, 32, 32},
    Origin: [3]float32{-16, -2, -8},
}),
```

Follow it with either [LightmapUVs](level-lightmap-uvs.md) plus
[ProductionBake](../generators/level-lightmap-bake.md), or `ProductionBake` with its `UVs` setting.
The latter lets independent receiver UV preparation and lighting-scene
preparation overlap when build execution allows concurrency.

[Plugin source](../../../pkg/level/build/chunks.go) · [Pipeline guide](../getting-started.md)
