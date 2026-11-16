# Processors

[Asset pipeline](../README.md) · [Get started](../getting-started.md)

Processors prepare an existing asset for use: resize an image, pack several
images into an atlas, reorder mesh vertices or lay out lighting coordinates.
For new surface maps, reflection captures, lightmaps and impostors, see
[Generators](../generators/README.md).

Most processors go in `Pipeline.Process`. Each guide shows the recipe placement
and the steps it needs before and after it.

## Images

| I want to… | Use | Place it… |
| --- | --- | --- |
| Reduce texture dimensions and build size | [Resize](image-resize.md) | After import, before other image processing |
| Share textures between icons or sprites | [Atlas](image-atlas.md) | After resizing or surface-map generation, before mipmaps |
| Reduce texture shimmering in the distance | [Mipmaps](image-mipmaps.md) | After image edits and atlas packing, before encoding |
| Convert a 360° reflection panorama to cube faces | [Reflection probes](image-reflection-probes.md) | After panorama import or capture, before KTX2 |

A typical texture recipe is **Images → Resize → Mipmaps → KTX2 → GoCatalog**.
You can insert [Materialize](../generators/image-materialize.md) or
[SSBump generation](../generators/image-ssbump.md) before mipmaps when you need
additional surface maps. ReflectionProbes prepares its own
face-safe mip levels, so it skips general Mipmaps.

## Meshes

| I want to… | Use | Place it… |
| --- | --- | --- |
| Remove unused vertices and empty triangles | [Cleanup](mesh-cleanup.md) | After PreparedGLB, before scene import or UV generation |
| Improve vertex reuse without changing the shape | [Optimize](mesh-optimize.md) | After cleanup/UV generation, before baking or publication |

For a baked level, cleanup happens after PreparedGLB and before scene import.
Optimization happens after the separate LightmapUVs step. The
[Sponza recipe](../../../samples/sponza/asset.config.go) uses both stages.

## Scene and lightmap preparation

These steps prepare geometry and material bindings for a later lighting bake.
They don't calculate finished lightmaps.

| I want to… | Use | Place it… |
| --- | --- | --- |
| Connect surface maps to a level's models | [Materials](level-materials.md) | After baked-lighting scene import and image processing |
| Split static geometry into loadable grid cells | [StaticChunks](level-static-chunks.md) | After Materials |
| Give each static level surface separate lighting pixels | [Level LightmapUVs](level-lightmap-uvs.md) | After StaticChunks, before optional Optimize |

Follow level preparation with the
[lightmap generator](../generators/level-lightmap-bake.md), then a
[level publisher](../publishers/level-publish.md). You can also let the lightmap
generator prepare its UVs internally when you don't need an optimization step
between unwrapping and baking.

## Terms used in these guides

- **Albedo** is a surface's base color before lighting. A **normal map** changes
  the surface directions used for lighting without adding geometry. **Ambient
  occlusion (AO)** describes small creases or enclosed areas that receive less
  surrounding light.
- **UV coordinates** map positions on a model to positions in a 2D texture.
  Lightmap UVs are a second, non-overlapping mapping, so separate surfaces can
  receive different baked shadows.
- A **lightmap** stores lighting calculated during the build. An ordinary RGB
  lightmap stores one color result. **Radiosity Normal Mapping (RNM)** stores
  three directional lighting maps so normal maps and relief can change the
  response at runtime. It trades more texture data for directional detail.
- **SSBump** means self-shadowed bump mapping. Its three coefficients describe
  small-scale surface direction and how the relief blocks incoming light. Those
  coefficients combine with RNM lighting; they are not ordinary RGB colors.
- An **atlas** packs multiple images or unwrapped surface pieces into one larger
  texture page. **Padding** is the extra border around a piece that protects it
  from neighboring pixels during filtering. **Mipmaps** are successively smaller
  texture levels used when the picture occupies less screen space.
- A lighting **receiver** is a covered surface sample for which the baker
  calculates illumination. Increasing texels per scene unit creates more of
  these samples, which costs bake time and texture memory.

For the libraries behind these operations, see
[offline tools and libraries](../tools.md).
