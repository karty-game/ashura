# Internal implementation

Public applications use [`pkg/`](../pkg/engine/README.md). These packages are
private implementation details; callers should not depend on their layout.

| Area | Ownership |
| --- | --- |
| `platform/browser`, `platform/native` | WebGPU resources, input, frame submission and platform services |
| `render` | Shared render records, geometry preparation and WGSL |
| `uipaint`, `spritepaint`, `diagnostics` | Retained UI/sprite submission formats and optional frame counters |
| `uihost` | Plain UI host events and attachment interfaces; no controls, fonts or shaping implementation |
| `*data`, `resourcedata` | Prepared runtime formats, bounded admission and resource ownership |
| `modelbuild`, `levelbuild`, `levelpack`, `lightmapbuild`, `impostorbake`, `ssbump` | Offline asset preparation and publication support |
| `lightmapper` | Production scene, transport and reconstruction preparation; `pkg/lightmap/bake` owns in-process Go GPU execution |
| `lightmapkernel` | Historical lightmap computation used by explicit compatibility APIs |
| `xatlas` | Embedded WASM UV generation, Go overlap/collapse repair and bounded geometry/chart admission |
| `basiscodec`, `msdfcodec` | Embedded offline WASM texture codecs and fixed-font distance-field generation through Go/wazero |
| `basistranscode` | Retained approved runtime ETC1S compatibility path, separate from offline GPU-ready exports |
| `webgpu` | Shared retained native WebGPU binding used by rendering and Go lightmap baking |
| `*fixture` and `testdata/` | Maintained regression inputs and their reproducible generators |

Runtime packages consume prepared data. Source image decoding, UV generation,
packing and lightmap computation belong to asset preparation. Go hosts the
embedded UV, texture and font libraries offline. Remaining native dependencies
retain their [tool/library ownership](../spec/asset-tools.md). The approved runtime
ETC1S compatibility path remains distinct; prepared native GPU exports do not
add runtime transcoding.

Tests and fixtures belong with the subsystem they validate. Runnable examples
and their authored artwork belong in [`samples/`](../samples/README.md).
Local investigations stay in ignored `tmp/` or `spikes/`, outside maintained
source and release gates. Preserving a public compatibility entry point does
not require retaining an unreachable private implementation behind it.
