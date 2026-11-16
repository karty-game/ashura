# Tools behind the pipeline

[Asset pipeline](README.md)

Most asset processing runs in Go. Where Ashura uses an existing encoder or UV
library, it embeds a pinned WebAssembly module and calls it through
[wazero](https://github.com/wazero/wazero), a WebAssembly runtime written in Go.
You don't need Node.js, Python or a native encoder installation to run those
plugins.

| Library or tool | Used for | How Ashura runs it |
| --- | --- | --- |
| [xatlas](https://github.com/jpcy/xatlas) | Unwrap model surfaces into lightmap UVs. | Embedded WASM, from [xatlas-wasi](https://github.com/kzhsw/xatlas-wasi). Used by [level lightmap UV preparation](processors/level-lightmap-uvs.md). |
| [meshoptimizer](https://github.com/zeux/meshoptimizer) | Reorder triangles and vertices to improve GPU cache use. | Embedded WASM for [Optimize](processors/mesh-optimize.md). [Cleanup](processors/mesh-cleanup.md) is Go code. |
| [Basis Universal](https://github.com/BinomialLLC/basis_universal) | Encode textures into compressed GPU formats. | Embedded WASM for compressed [KTX2](publishers/image-ktx2.md), [sprite pages](publishers/sprite-images.md), impostors and level output. |
| [msdfgen](https://github.com/Chlumsky/msdfgen) | Generate distance fields that keep text edges sharp when scaled. | Embedded WASM for the fixed Go Regular [default font](generators/font-default.md). |
| [Materialize](https://github.com/maikramer/AiGameKit/tree/1a3fe7d052a464d3217ef66d2bce9889d3946792/Materialize) | Generate normal and ambient-occlusion maps from color images. | Native GPU CLI, pinned to 2.0.0, for the [Materialize plugin](generators/image-materialize.md). |
| [wgpu-native](https://github.com/gfx-rs/wgpu-native) | Run the GPU compute work used by the lightmapper. | Shared native library through Ashura's existing Go binding, for [ProductionBake](generators/level-lightmap-bake.md). |

The encoders, UV unwrapper and font generator are build tools; they aren't part
of your game. Texture encoding, atlas packing and mip generation finish before loading.
Some [level publication options](publishers/level-publish.md) can explicitly select
load-time transcoding; use native texture profiles for GPU-ready delivery.

## Formats aren't tools

[glTF/GLB](https://www.khronos.org/gltf/) is the source model format supported by
the [model importers](importers/model-glb.md). Ashura reads its supported static subset
in Go and writes prepared model data.

[KTX2](https://www.khronos.org/ktx/) is a texture container. A `.ktx2` file can hold
uncompressed RGBA8 or compressed blocks, depending on the recipe. Ashura writes
the container in Go and uses Basis Universal when compression is requested;
it doesn't invoke the KTX command-line tools.

## What needs installing?

Run `mise install` for the repository's pinned tools. The embedded WASM libraries
need no separate installation. Materialize and GPU lightmap baking also need a
working graphics driver/backend on the build machine. An ordinary image resize,
atlas pack, model import or impostor capture doesn't need a GPU bake device.

For exact module versions, licenses and checksums, see the retained notices for
[xatlas](../../internal/xatlas/third_party/README.md),
[meshoptimizer](../../internal/meshopt/third_party/README.md),
[Basis Universal](../../internal/basiscodec/third_party/README.md) and
[msdfgen](../../internal/msdfcodec/third_party/README.md). The
[tool requirements](../../spec/asset-tools.md) cover the native dependencies.
