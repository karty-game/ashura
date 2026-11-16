# Offline mesh optimization

`mesh/build` provides the cleanup and vertex-ordering stages used by the level
pipeline. Games import `mesh`, never this build package. Output remains ordinary
GPU-ready geometry; no runtime mesh decoder is added.

## Recipe order

The [Sponza recipe](../../../samples/sponza/asset.config.go) uses:

```text
PreparedGLB → Cleanup → Scenes → Materials → StaticChunks
            → LightmapUVs → Optimize → ProductionBake → PublishVariants
```

Put cleanup after prepared model import and before `levelbuild.Scenes`:

```go
Import: []assetbuild.Plugin{
    modelbuild.PreparedGLB(),
    meshbuild.Cleanup(),
    levelbuild.Scenes(levelbuild.SceneOptions{
        Input: []string{"levels/main.scene.yaml"},
    }),
},
```

Put optimization after separately staged production UV preparation, before the
baker. Leave `ProductionBake.UVs` nil in this form:

```go
Process: []assetbuild.Plugin{
    levelbuild.Materials(levelbuild.MaterialOptions{}),
    levelbuild.StaticChunks(levelbuild.ChunkOptions{}),
    levelbuild.LightmapUVs(levelbuild.UVOptions{Production: true}),
    meshbuild.Optimize(),
    // ProductionBake follows here.
},
```

These are fragments for an `assetbuild.Pipeline`. Import `assetbuild`,
`modelbuild`, `levelbuild` and `meshbuild` from their corresponding
`github.com/karty-game/karty-webgpu/pkg/` packages.

## Cleanup

`Cleanup()` removes unused vertices, deduplicates byte-identical
position/normal/UV0 records within each material part, and removes exactly
zero-area triangles. Material boundaries, hard normals and UV seams stay
separate. Small nonzero geometry is retained. An entirely degenerate part is
dropped; a model with no usable geometry fails preparation. Cleanup does not
repair arbitrary broken topology or remove overlapping faces.

## Optimization

`Optimize()` uses upstream meshoptimizer 1.0 vertex-cache ordering followed by
vertex-fetch remapping. Surviving oriented triangles and vertex attributes are
preserved. Triangle order and vertex indices may change; no overdraw pass,
quantization or simplification is applied.

The level adapter moves secondary UVs with the vertices and updates model
checksums and receiver references before baking. Atlas layout, texel density,
padding and lightmap resolution remain the same. Cleanup may change charting;
GPU ordering alone does not guarantee better lighting or a measured FPS gain.

## Portable implementation

Cleanup is Go code. Optimization runs the pinned upstream WASM library through
wazero, with a 256 MiB guest-memory ceiling and cancellation. It needs no CGO,
JavaScript engine, native compiler or tool installation. Indices and finite
attributes are checked before guest execution; returned remaps are bounded and
checked before buffers are created. Each action closes its VM explicitly.

The upstream commit, hashes, license and extraction tool are retained under
[meshoptimizer notices](../../../internal/meshopt/third_party/README.md).
