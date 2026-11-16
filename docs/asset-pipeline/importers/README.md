# Importers

[Asset pipeline](../README.md)

Choose the importer for your source and the output you need. Processors prepare
its data, generators derive other assets, and publishers write the result.

| I have… | Use | Result |
| --- | --- | --- |
| PNG or JPEG artwork | [Image import](image-import.md) | Images ready for resizing, atlases or texture encoding. |
| A static GLB to load directly | [GLBWithOptions](model-glb.md) | A prepared model with an embedded Go accessor. |
| GLB models for a level or an impostor-only recipe | [PreparedGLB](model-prepared-glb.md) | Prepared models consumed by the downstream level or impostor plugins. |
| YAML scenes and prefabs | [Level scenes](scene-lightmap-baking.md) | Gameplay constructors and static geometry for chunking and lighting preparation. |

`GLBWithOptions` can reduce embedded texture resolution. `PreparedGLB` keeps the
authored texture resolution. Both read the same supported subset of
[glTF/GLB](https://www.khronos.org/gltf/).

```mermaid
flowchart LR
    Source[Static GLB] --> Choice{What do you need?}
    Choice -->|Load directly| GLB[GLBWithOptions]
    GLB --> Embedded[Embedded model and Go accessor]
    Choice -->|Bake a level or proxy| Prepared[PreparedGLB]
    Prepared --> Level[Scene import and level preparation]
    Prepared --> Proxy[Impostor cards]
```

`levelbuild.Scenes` reads `.scene.yaml` and expands reusable prefabs. It generates
Go constructors for logic and dynamic objects while preparing static models for
the level pipeline. Models referenced by YAML must be imported earlier in the
same pipeline. Custom components use the shared
[scene module API](../../../pkg/scene/build/types.go).

To capture reflections from a room made of planes, boxes and spheres, use the
[reflection generator](../generators/scene-reflections.md). It reads its own
JSON format and produces a panorama and typed room data.

Next: [write a recipe](../getting-started.md), choose a
[processor](../processors/README.md), a [generator](../generators/README.md),
or a [publisher](../publishers/README.md).
