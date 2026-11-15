# Static GLB model profile

GLB is an authoring format. The build-only `model/build.GLB()` asset plugin
prepares GPU-ready geometry/textures and named generated declarations. Games
import `model`, not the build package. A recipe uses:

```go
assetbuild.Config{
    Output: "generated",
    Pipelines: []assetbuild.Pipeline{{
        Name: "models", Input: []string{"assets/**"},
        Import: []assetbuild.Plugin{modelbuild.GLB()},
    }},
}
```

Generated functions in package `models` return `model.Asset`; for example
`models.AssetsScene().Load(app)` loads `assets/scene.glb`'s prepared embed.
The original source identity is diagnostic metadata, never a filesystem lookup.
The explicit single-file command is also available:

```sh
engine assets model --input source.glb --output prepared.kmodel
```

```go
asset, err := models.AssetsScene().Load(app)
// Handle err.
instance, err := asset.Instantiate(world, engine.DefaultTransform())
// Handle err; instance.Entities() exposes ordinary ECS handles.
instance.Remove()
return asset.Unload()
```

Loading first admits the complete bounded container, every primitive and every
texture before creating App-owned resources. It copies each admitted packed geometry and texture payload once for ownership,
retaining the exact GPU bytes without keeping a duplicate prepared texture container. There is
no runtime GLB/JSON/image decoding, node traversal, normal generation, geometry
conversion, channel packing or mip generation. Geometry reaches WebGPU in its
authored 32-byte interleaved vertex and uint32 index layout without repacking or
revalidating its contents on normal frames. Prepared KTX2 descriptor/bounds and
RGBA8 mip admission remain required. Root instance transforms are ordinary ECS
values; baked primitive transforms require no permanent scene hierarchy.

For immutable external resources, `resource.ReadVerified` returns an opaque
payload after the same complete size/SHA admission as `resource.Read`.
`model.Verified(name, payload)` reuses that exact checksum and immutable geometry
storage. It does not hash the model or copy its vertex/index bytes again.
Finite values, index ranges, materials and UV-to-model binding are still checked
before App resources are created. A zero payload is not verified. No public
mutable byte accessor exists on the payload; ordinary `model.Prepared` retains
its caller-buffer copying and checksum behavior. Caller-supplied UV streams
retain their normal validation and copy semantics in either path.

Instance root transforms must be finite and preserve orientation: positive scale
determinant, or the engine's all-zero scale shorthand for unit scale. Singular or
reflecting roots fail before creating entities. Two negative scale axes preserve
orientation and are supported. Bake authored reflections into GLB nodes, where
winding is corrected offline. Later direct writes to an instance entity's scale
must preserve this v1 contract; dynamic reflected-root culling is outside v1.

Each logical instance contains all selected-scene primitive entities. Removing
one instance preserves other users. Unload rejects live model instances and
preflights all texture references atomically. Manual `World.Remove` of individual
entities is supported; dead instance metadata is pruned on later instantiate or
unload. App.Close invalidates later loading/instantiation and releases GPU, texture and
owned CPU geometry bytes through an explicit lifecycle callback. Unload unregisters
that callback, so repeated load/unload does not retain historical models. Methods follow the App's single loop-goroutine mutation contract.

Supported source profile:

- GLB 2.0 with one embedded BIN buffer and an explicit/default selected scene.
- Static indexed or unindexed TRIANGLES; uint8/uint16/uint32 source indices.
- Float32 POSITION, NORMAL and TEXCOORD_0, including valid interleaved accessors.
- Multiple primitives/materials and node hierarchy; affine matrices or TRS.
  All node transforms are baked offline, normals use inverse-transpose transforms,
  and negative determinants reverse triangle winding. Missing normals become
  flat geometry offline; missing UVs are allowed only without mapped textures.
- Opaque base-color tint (source tint alpha and alphaCutoff are ignored as
  glTF requires), lightweight roughness, embedded PNG/JPEG albedo,
  normal scale and red-channel AO strength. Color mips use linear-light filtering,
  normal mips renormalize and AO mips average linear values, entirely offline.
- MASK cutouts preserve tint/image alpha and an explicit cutoff (0.5 when omitted),
  with source alpha retained in offline-generated mips. Opaque-only containers
  retain exact V1 bytes; masks select [prepared model V2](../../spec/model-v2.md).
  Blended transparency remains unsupported. Very thin masks need distant-mip
  quality review; current alpha mips average without threshold-specific adjustment.
- Default single-sided and explicit double-sided materials preserve culling state.
  The shader is the library's lightweight Standard model, not full glTF PBR.
- Repeat sampling and linear mip filtering. Undefined sampler state selects that
  supported portable default.

Rejected explicitly: animations, skins, morph targets/weights, cameras/lights,
extensions (including compression/texture transforms), sparse/normalized or
non-float vertex attributes, supplied tangents, vertex colors, extra UV sets,
external/data-URI buffers/images, emissive data, metallic workflows and BLEND or
unknown alpha modes. Explicit glTF materials must author `metallicFactor: 0`; glTF's
omitted metallic default of one is rejected, including an omitted primitive material. Unsupported sampler wrapping/filter
state is rejected. Tangents are derived by Standard from UV derivatives; v1 does
not silently discard authored tangent attributes.

Bounds: 64 MiB GLB, 8 MiB JSON, 128 hierarchy/JSON depth, 4096 nodes/parts,
64 prepared role-specific textures, aggregate 1,048,576 output vertices and
3,145,728 indices, 4096-pixel textures and 128 MiB prepared model container.
Counts, finite data, index/accessor ranges, graph cycles/multiple parents, texture
roles and material references are checked before ownership/GPU admission.
No external-resource path is opened. Explicit source/output symlinks and parent
symlinks are rejected; failed compilation preserves existing prepared outputs.
Prepared bytes/catalogs stay ignored and rebuild from source assets.

## Prepared lightmap UVs

Use build-only `modelbuild.LightmappedGLB(modelbuild.LightmapOptions{Resolution:1024, Padding:2})`
to emit a remapped model and a matching `.klightmap` declaration. The build
module calls embedded xatlas WASM through Go/wazero; ordinary recipes need no
compiler, executable path or separate tool installation. Go repairs invalid
charts before complete geometry, density and receiver admission. The former
`LightmapOptions.Compiler` and `Executable` fields have been removed. UV
preparation always uses the embedded library. See
[UV preparation](../../spec/asset-tools.md#embedded-xatlas-preparation) and
[the current decision](../../spec/decisions/0015-portable-offline-libraries.md).

Load the generated lightmap inputs with `models.AssetsSponzaLightmap().Load()`, then call
`models.AssetsSponza().LoadLightmapped(app, inputs)`. Admission checks the exact
model digest and every secondary UV stream before creating textures. UV1 uploads
as a separate immutable vertex stream; the ordinary model@1 vertex ABI stays 32 bytes.
Lighting values are supplied separately through Standard's linear lightmap
texture and decoding scale. See [lightmap slice](../../spec/lightmap-v1.md).


## Presenting existing prepared ranges

`Model.InstantiateRange(world, transform, first, count)` creates a presentation
from a contiguous subset of the model's prebuilt primitive records. It shares
the existing geometry and textures without slicing vertex/index data, converting
meshes or producing new draw ranges. Invalid bounds fail before changing the
world. Remove the returned instance explicitly when no longer wanted; model
unload continues to reject live users.

Range presentation avoids scanning historical instances on every bounded attach.
The original full `Instantiate` retains its existing historical-instance pruning,
and `Unload` still performs lifetime admission. The baked-level presenter uses
this API to expose at most eight prebuilt material ranges per update while
keeping logical scene ownership independent of presentation.

## Demo texture profiles

Offline recipes can cap embedded model images without altering source GLBs or
geometry. Use `modelbuild.GLBWithOptions(modelbuild.GLBOptions{MaxTextureDimension: 512})`
instead of `GLB()`. The cap preserves aspect ratio, never upscales, and precedes
role-aware mip generation. Albedo filtering uses linear light; normal maps are
renormalized. Zero keeps authored dimensions; supported caps are 1 through 4096.
The option participates in asset caching. Both a full model and an impostor baked
from its prepared slot see the same selected texture profile; no runtime resize
or conversion is introduced.
