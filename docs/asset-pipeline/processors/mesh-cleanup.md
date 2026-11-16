# Clean up meshes

[Asset pipeline](../README.md) · [Processors](README.md)

Use `meshbuild.Cleanup()` to remove unused geometry left in an exported model:
vertices that no triangle uses, exact duplicate vertices and triangles with
zero area. Cleaning before lightmap UV generation can also avoid spending work
on empty faces.

Cleanup works within each material part. It keeps hard surface edges, texture
seams and small triangles with nonzero area. It does not reduce a model's detail
or merge nearby points just because they are close together.

Import `meshbuild "github.com/karty-game/karty-webgpu/pkg/mesh/build"`.

## Settings and defaults

There are no options. All prepared models in the pipeline are processed;
unrelated images and other assets are skipped. A part with no usable triangles
is removed; a model with no usable geometry fails the build. Cleanup does not
repair overlapping faces or arbitrary broken surface topology.

Cleanup changes the current resources for later stages; it publishes no files.
The implementation is Go and adds no runtime decoder or compression format.

## How to use it

The maintained level path runs cleanup after
[PreparedGLB](../importers/model-prepared-glb.md), before scene import and UV
generation. Import `modelbuild` from `pkg/model/build` and `levelbuild` from
`pkg/level/build` under `github.com/karty-game/karty-webgpu/`:

```go
Import: []assetbuild.Plugin{
    modelbuild.PreparedGLB(),
    meshbuild.Cleanup(),
    levelbuild.Scenes(levelbuild.SceneOptions{
        Input: []string{"levels/main.scene.yaml"},
    }),
},
```

Put this fragment in your [recipe](../getting-started.md) and declare the YAML
and its GLBs in `Pipeline.Input`. Continue with materials, chunks and
[lightmap UVs](level-lightmap-uvs.md). Run [Optimize](mesh-optimize.md) after UV
preparation and before the lighting bake. The
[Sponza recipe](../../../samples/sponza/asset.config.go) shows the complete flow.

[Package guide](../../../pkg/mesh/build/README.md) ·
[Go implementation](../../../pkg/mesh/build/build.go)
