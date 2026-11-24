# Reflection room

This sample demonstrates one placed local probe serving reflective materials
on the floor, four walls and ceiling. Moving the camera shows cubemap parallax
correction against the probe's box bounds. The orange block, blue sphere and red
pillar provide recognizable reflection landmarks.

[assets/room.json](assets/room.json) owns the shared lighting and probe placement.
[assets/chunks/room.json](assets/chunks/room.json) owns the room's geometry,
colors and transforms. The recipe generates the visible scene and its offline
panorama from those same records, so the reflected surfaces and props stay
consistent. JSON decoding and scene generation run only during asset preparation.
The sample contains no custom source-processing or code-generation package.

The asset recipe imports a 512×256 equirectangular image through the shared
[reflection build module](../../pkg/reflection/build/README.md), then runs `ReflectionProbes()`,
`KTX2Variants()` and `GoCatalog()`. The panorama is a small analytic diffuse bake from the
probe position, not a capture of the engine renderer. It uses the shared lights
and material colors, with ideal sphere intersections; recursive reflections and
view-dependent specular highlights are excluded. It remains a static environment
until the assets are rebuilt.

`ReflectionProbes()` projects that image into a cubemap atlas and filters each
face's mip levels offline. The resulting faces are 128×128 before mip filtering.
KTX2 output stores the opaque reflection color in sRGB BC1 and ETC2 RGB,
preserving the atlas dimensions and all filtered mip levels. Encoding runs
offline. The sample selects a supported format and uploads its native blocks
directly.

### Chunk-local incremental baking

The shared build module uses 32 m chunks with grid origin `[-16, -16, -16]`. The probe bake
uses its current chunk and the immediate neighbors, including diagonals, in
the same `space`. Capture rays stop at 32 m; a miss uses the shared sky fill
instead of leaving transparent pixels. The shared sky and directional key are
global inputs, so edits to either rebake the probe regardless of chunk distance.
Recursive reflections and view-dependent highlights remain excluded.

Each file in `assets/chunks/` has a `space`, integer `cell` and `objects` array.
An object's complete rotated bounds must fit inside its cell; crossing geometry
must be split between cells. Shapes use `0` for plane, `1` for box and `2` for
sphere. The sample's nine objects all fit in cell `[0, 0, 0]`.

Nearby chunk edits invalidate the panorama action. Distant chunk edits update
the generated visible scene while preserving the cached probe image and its
timestamp. File membership is rediscovered on each build, so adding, moving or
removing a neighbor also changes the bake inputs. Overlapping chunks from another
`space` do not contribute. This is a local reflection approximation, not a
whole-level visibility or global illumination solution.

Scene data is kept outside Go source so authored edits can reuse individual
actions without changing the recipe executable. Renderer or recipe code edits
still invalidate their cache normally. Rebuild after editing the room or bake:

```sh
mise exec -- go run ./cmd/engine assets generate --project ./samples/reflections
# or
mise exec -- go run ./cmd/engine assets build --project ./samples/reflections
# Force all processing instead of reusing cached actions:
mise exec -- go run ./cmd/engine assets build --project ./samples/reflections --no-cache
```

Reflections start enabled; use **Reflections** to compare the same scene without
the probe contribution. **Surface settings** exposes roughness. Diagnostics start
collapsed. Click to look and use WASD to move.

Then run the sample:

```sh
mise exec -- go run ./cmd/engine run web ./samples/reflections
```

See [the reflection API](../../pkg/reflection/README.md) for probe bounds, overlap
selection and the bake format.

`main.go` owns the common `main`/`run` lifecycle. `scene.go` loads the prepared
environment, selects the camera and creates meshes from generated `RoomScene()`.
Startup and cleanup errors return through `run` and are logged once by `main`.
The runtime only loads the prepared GPU payload; panorama generation, cubemap
projection and mip filtering all belong to the asset recipe.
