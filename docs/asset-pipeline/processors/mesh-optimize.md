# Optimize meshes

[Asset pipeline](../README.md) · [Processors](README.md)

Use `meshbuild.Optimize()` to arrange a model's triangles and vertices so the GPU
can reuse recently read vertex data. The model keeps its shape, triangle winding
and texture coordinates. The output uses ordinary vertex and index buffers,
with no decompression step in the game.

Import `meshbuild "github.com/karty-game/ashura/pkg/mesh/build"`.

## Settings and defaults

There are no options. The stage processes prepared models and lighting geometry;
unrelated assets are skipped. It uses
[meshoptimizer](https://github.com/zeux/meshoptimizer) 1.0 to reorder triangles for
the vertex cache, then reorder vertex storage to match. It does not simplify
geometry, reduce numeric precision or reorder for overdraw.

The embedded WASM implementation runs offline through Go/wazero. It requires no
native tool installation. The stage updates resources for subsequent processors
or publishers and publishes no files itself. Actual frame-rate gains depend on
the workload and require GPU measurement.

## How to use it

For levels, place optimization after the separate lightmap UV stage and before
baking. Import `levelbuild` from
`github.com/karty-game/ashura/pkg/level/build`:

```go
Process: []assetbuild.Plugin{
    levelbuild.Materials(levelbuild.MaterialOptions{}),
    levelbuild.StaticChunks(levelbuild.ChunkOptions{}),
    levelbuild.LightmapUVs(levelbuild.UVOptions{
        Production: true, TexelsPerUnit: 8, MaxPageSize: 512, Padding: 4,
    }),
    meshbuild.Optimize(),
    // ProductionBake follows with UVs left nil.
},
```

Run [Cleanup](mesh-cleanup.md) after prepared model import and before
[scene import](../importers/scene-lightmap-baking.md). The level pipeline updates
geometry and lighting coordinates together. Use the separate UV stage so the
optimizer can run between unwrapping and baking.

See the [Sponza recipe](../../../samples/sponza/asset.config.go) for the complete
input, bake and publication settings.

[Package guide](../../../pkg/mesh/build/README.md) ·
[Go implementation](../../../pkg/mesh/build/build.go)
