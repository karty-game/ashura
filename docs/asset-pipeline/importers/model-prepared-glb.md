# Import models for level preparation

[Asset pipeline](../README.md) · [Importers](README.md)

Use `modelbuild.PreparedGLB()` when a level or impostor plugin will consume the
model. It prepares a static GLB and keeps the result inside the build pipeline;
it writes no game files by itself. For a model to embed and load directly, use
[GLBWithOptions](model-glb.md).

Import `modelbuild "github.com/karty-game/karty-webgpu/pkg/model/build"`.

## Settings and defaults

There are no plugin options. `Pipeline.Input` supplies project-relative paths
or patterns; only `.glb` files are selected, case-insensitively. Embedded
textures retain their authored resolution, with mipmaps prepared offline. The
supported GLB profile and limits are the same as [GLBWithOptions](model-glb.md).

Later plugins refer to each model by its project-relative source path, such as
`assets/building.glb`. Keep the importer and its consumers in the same pipeline.

## How to use it

This starts a [level recipe](scene-lightmap-baking.md). Import `levelbuild` from
`github.com/karty-game/karty-webgpu/pkg/level/build` and `meshbuild` from
`github.com/karty-game/karty-webgpu/pkg/mesh/build`:

```go
Import: []assetbuild.Plugin{
    modelbuild.PreparedGLB(),
    meshbuild.Cleanup(),
    levelbuild.Scenes(levelbuild.SceneOptions{
        Input: []string{"levels/main.scene.yaml"},
    }),
},
Process: []assetbuild.Plugin{
    levelbuild.Materials(levelbuild.MaterialOptions{}),
    levelbuild.StaticChunks(levelbuild.ChunkOptions{}),
    // Add UV preparation, optimization and lighting bake here.
},
```

Declare the referenced GLBs and YAML in `Pipeline.Input`. See the
[Sponza recipe](../../../samples/sponza/asset.config.go) for the full sequence.
For proxy-only output, use `PreparedGLB` directly before
[impostor cards](../generators/model-impostors.md); that generator publishes its
own asset without embedding the full source model.

[Go implementation](../../../pkg/model/build/prepared.go)
