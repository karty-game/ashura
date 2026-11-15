# Materials: Standard and Valve / Source

Compare the two material models on the same brick cube, brick sphere and tiled
stone ground. The camera, meshes, textures and raking directional light stay in
place when switching. **Valve / Source material** is a visible WebGPU UI switch;
**T** selects the other mode from the keyboard. The optional debug panel starts
hidden so the comparison controls remain visible; use its Debug button or F3.

```sh
mise exec -- go run ./cmd/engine run web ./samples/standard-materials
```

The CLI prepares the explicit project recipe, builds WASM, and prints a free
loopback URL. Open that URL or forward its port. Drag the scene outside the
controls to look; WASD moves and Space/Shift moves vertically. The bounded control
card keeps a usable cursor, scrolls on short viewports and leaves scene input
outside its bounds available. The controls use the shared UI library rather than
DOM elements.

| Mode | Controls | Shading |
| --- | --- | --- |
| Standard | N: normal mapping; O: ambient occlusion | Tangent-space normals, authored AO and a lightweight specular term |
| Valve / Source | B: SSBump relief; H: Half-Lambert comparison; R: RNM ambient arrangements | Diffuse SSBump/RNM with precomputed directional visibility |
| Either | T or the visible material switch | Same geometry, view and shared albedo handles |

Inactive controls are disabled. Each mode retains its comparison settings when
switching away and back. R cycles neutral ambient fill and three colored
arrangements; H is an optional wrapped-light comparison, disabled by default.
The sphere repeats its maps 3×3, making curved-surface differences easy to see.

The mode switch and explanation stay visible in a compact bottom panel.
**Comparison settings** expands map and RNM controls. Diagnostics start collapsed;
the camera adjusts its field of view when the viewport changes.

## Code and lifetime

The [shared sample conventions](../README.md#code-and-asset-conventions) apply.
`main.go` owns the App lifecycle, `scene.go` creates the camera/geometry/lights,
`assets.go` loads shared prepared material pairs, `comparisons.go` switches their
ECS components, and `interface.go` builds the controls. Initial camera selection,
setup, callback and cleanup failures are checked consistently.

The default scene starts in Standard mode. Each albedo, normal, AO and SSBump
texture is loaded once for its material. Both descriptors share the albedo handle;
mode changes remove the inactive component instead of retaining competing
providers. Meshes and textures are neither rebuilt nor reloaded. Mode/toggle
changes update only those three surface components, never scan every frame.

## One explicit asset pipeline

`assets/` retains the six original 256×256 PNGs: brick and stone albedo, normal
and AO. The four images formerly copied into the separate Source sample were
byte-identical duplicates; the combined sample has one authored copy of each.
`asset.config.go` owns generation before development and asset preparation for
the playable build. Its named `standardMaterials` pipeline imports the selected
images, processes normal/SSBump maps and mips, then outputs KTX2 and Go catalogs.
The optional Materialize pass stays in that same pipeline so its derived normal
records feed SSBump directly.

The default `dev` and `release` recipe is:

```text
Images → SSBump (KeepNormals: true) → Mipmaps → KTX2 → GoCatalog
```

KTX2 output uses sRGB BC1 for albedo and linear BC7 for normal and SSBump maps.
AO retains RGBA8. Compression runs offline with the original 256×256 dimensions
and complete authored mip chains; loading uploads native blocks directly. The
sample requires the device's BC texture compression feature.

SSBump retains each normal for Standard and derives a separate linear RNM map
for Valve / Source. Albedo/AO inputs pass through unchanged. Generated functions
include `assets.BrickAlbedo()`, `BrickNormal()`, `BrickAO()` and `BrickSSBump()`
(and their Stone counterparts). Runtime code loads only these declarations.
`generated/assets/` contains ignored, disposable GPU-ready payloads and catalog
code; regenerate it rather than editing it:

```sh
mise exec -- go run ./cmd/engine assets generate --project ./samples/standard-materials
# Refresh playable assets after development:
mise exec -- go run ./cmd/engine assets build --project ./samples/standard-materials
```

Albedo mips use linear-light filtering, normal mips are renormalized, and AO and
SSBump mips average their linear values. The SSBump stage computes squared RNM
projections and directional visibility from periodic normal-map relief offline.
Its default bake uses 300 deterministic rays, distance 32 texels and bias 0.05.
These are demonstration estimates from the retained normals, not original
measured geometry or Valve-exported assets. Runtime loading/rendering performs
no source decoding, occlusion bake, channel conversion or mip generation.

The optional `materialize` profile replaces authored normals with generated
normal estimates while retaining authored albedo and AO:

```sh
mise exec -- go run ./cmd/engine assets build --project ./samples/standard-materials --profile materialize
```

It imports only albedo/AO inputs, generates normals from albedo, retains those
normals and derives SSBump from them. The module uses required mise to install
its checksum-pinned prebuilt tool; no executable path or native compiler is part
of sample setup. The published Materialize 2.0.0 Windows package requires the
Microsoft Visual C++ runtime. Actual GPU map generation requires a
graphics backend. An optional declared backend identity enables result caching;
omitting it reruns Materialize. Use `--profile dev` to restore the
authored-normal outputs. No source image is overwritten by either profile.

## Material interpretation

Standard AO attenuates ambient fill. Source's RGB SSBump coefficients are
unsigned linear lighting weights along three fixed tangent-space RNM directions;
weights are neither signed-decoded nor normalized, preserving baked occlusion.
Source ignores the hemisphere light and uses its three `AmbientColors` instead.
It has no separate AO lookup or specular term. Neither mode bakes a cube's scene
shadow onto the ground, displaces geometry or supplies a full PBR model.

The representation follows Valve's
[Efficient Self-Shadowed Radiosity Normal Mapping paper](https://cdn.steamstatic.com/apps/valve/2007/SIGGRAPH2007_EfficientSelfShadowedRadiosityNormalMapping.pdf)
and its [Source SDK RNM shader](https://github.com/ValveSoftware/source-sdk-2013/blob/master/src/materialsystem/stdshaders/lightmappedgeneric_ps2_3_x.h).
