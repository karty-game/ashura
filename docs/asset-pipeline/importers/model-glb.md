# Import GLB models

[Asset pipeline](../README.md) · [Importers](README.md)

Both model import functions read an ordinary `.glb` source file, such as a model
you downloaded or exported from a modeling tool. During the build, Ashura
applies node transforms and converts its geometry and textures into data the
game can load directly.

Choose what happens to that converted result:

| I want to… | Use | Result |
| --- | --- | --- |
| Embed the model and load it in the game | `modelbuild.GLBWithOptions(...)` | Writes model data and a Go catalog that embeds it. |
| Feed the model into a level bake or impostor generator | `modelbuild.PreparedGLB()` | Keeps the converted model inside the build pipeline for later plugins, without publishing the original model. |

The word “prepared” describes Ashura's intermediate output. The input to both
functions is the same ordinary GLB format.

Import `modelbuild "github.com/karty-game/karty-webgpu/pkg/model/build"`.

## Settings and defaults

| Setting | Default | Meaning |
| --- | --- | --- |
| `Pipeline.Input` | No inputs | Project-relative paths or patterns. Both functions select `.glb` files, case-insensitively. |
| `GLBOptions.MaxTextureDimension` | `0` | For `GLBWithOptions`: keep authored texture dimensions. A positive value caps either dimension, preserves aspect ratio and never upscales. Range: 0–4096. Applied before mip generation. |

Pass `modelbuild.GLBOptions{}` to keep authored texture dimensions with
`GLBWithOptions`. `PreparedGLB` has no options and keeps authored texture
resolution. Both generate texture mipmaps during the build.

## Embed the model in the game

Place `GLBWithOptions` in your [recipe](../getting-started.md):

```go
assetbuild.Pipeline{
    Name: "models",
    Input: []string{"assets/*.glb"},
    Import: []assetbuild.Plugin{
        modelbuild.GLBWithOptions(modelbuild.GLBOptions{
            MaxTextureDimension: 512,
        }),
    },
}
```

For `assets/rock.glb`, output under `Config.Output` contains
`models/model_AssetsRock.kmodel` and `models/model_AssetsRock.go`. The generated
`models` package provides `models.AssetsRock()`, returning a `model.Asset`.
Source paths determine the Go names; name collisions fail the build.

For a game module named `example.com/game`, import
`"example.com/game/generated/models"`, then call
`models.AssetsRock().Load(app)`. The accessor embeds the converted model; the
game doesn't open the source GLB or decode its source images.

## Pass the model to later build steps

Use `PreparedGLB` when a level or impostor plugin will consume the converted
model. The maintained level path starts like this. Import `levelbuild` from
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

Declare the referenced GLBs and YAML in `Pipeline.Input`. Later plugins refer
to models by their source path, such as `assets/building.glb`. Keep these steps
in the same pipeline. [Level scene import](scene-lightmap-baking.md) and the
[Sponza recipe](../../../samples/sponza/asset.config.go) show the full workflow.

For proxy-only output, use `PreparedGLB` directly before
[impostor cards](../generators/model-impostors.md). That generator publishes its
own asset without embedding the full source model. `GLBWithOptions` instead
publishes the original model during import, so later processing doesn't replace
that already-published copy.

## Supported GLB files

[GLB is the binary form of glTF](https://www.khronos.org/gltf/). Ashura supports
static GLB 2.0 models with embedded images and lightweight Standard materials.
Opaque and MASK materials are supported. Animation, skins, BLEND, external
images, extensions and full metallic/PBR workflows are rejected. Check the
[complete profile and limits](../../../pkg/model/README.md) when choosing or
exporting a model.

[Go implementation](../../../pkg/model/build/glb.go)
