# Import static GLB models

[Asset pipeline](../README.md) · [Importers](README.md) · Static models

Use `modelbuild.GLBWithOptions(options)` to turn a static model into an asset your game can load
directly. The build applies node transforms, prepares geometry and embedded
textures, and creates smaller texture levels for distant views. It writes the
prepared model and a Go function that embeds it.

[GLB is the binary form of glTF](https://www.khronos.org/gltf/). Ashura supports
a static subset of that format, described below.

Import `modelbuild "github.com/karty-game/karty-webgpu/pkg/model/build"`.

## Settings and defaults

Pass `modelbuild.GLBOptions{}` to keep authored texture dimensions.

| Setting | Default | Meaning |
| --- | --- | --- |
| `GLBOptions.MaxTextureDimension` | `0` | Keep authored resolution. A positive value caps either dimension, preserving aspect ratio, before mip generation. Accepted range: 0–4096. |
| `Pipeline.Input` | No inputs | Project-relative paths or patterns. Only `.glb` files are selected, case-insensitively. |

For each source, the plugin writes `models/model_<Name>.kmodel` and
`models/model_<Name>.go` below `Config.Output`. The generated package is `models`.
For `assets/rock.glb`, it provides `models.AssetsRock()`, returning a `model.Asset`.
Names include the source path; two sources that produce the same Go name fail
the build.

## How to use it

Place the plugin in `Import` in your [recipe](../getting-started.md):

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

For a game module named `example.com/game`, import
`"example.com/game/generated/models"`, then call
`models.AssetsRock().Load(app)`. The generated asset contains the prepared bytes;
the game does not open the original GLB or decode its source images.

## When to use another importer

Use [PreparedGLB](model-prepared-glb.md) for a level bake or an impostor-only
recipe. It leaves publication to those downstream plugins. `GLBWithOptions`
publishes its own embedded model while importing; later geometry changes do
not replace that published copy. Texture resizing belongs to `GLBWithOptions`;
`PreparedGLB` has no texture-cap option.

Supported models use embedded GLB 2.0 data and the engine's lightweight Standard
materials. Opaque and MASK materials are supported. Animation, skins, BLEND,
external images, extensions and full glTF metallic/PBR workflows are rejected.
See the [complete supported profile and limits](../../../pkg/model/README.md)
before exporting a new model.

[Go implementation](../../../pkg/model/build/glb.go)
