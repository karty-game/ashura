# Generate reflections from a primitive scene

[Asset pipeline](../README.md) · [Generators](README.md)

`reflectionbuild.Scene` describes a room made of planes, boxes and spheres,
then renders a panorama during the build. Use it for small analytic scenes like
the reflection sample. It writes typed Go scene data as well as the panorama.

The capture code runs in Go without a GPU capture device or an external tool.
For reflections from an authored panorama, start with [Images](../importers/image-import.md) and
[ReflectionProbes](../processors/image-reflection-probes.md) instead.

Import `reflectionbuild "github.com/karty-game/ashura/pkg/reflection/build"`.

## Settings and defaults

All three `reflectionbuild.SceneOptions` fields are required:

| Setting | Meaning |
| --- | --- |
| `Name` | Exported Go identifier, such as `Room`. Generates `RoomScene()` and a `RoomReflection` texture name. |
| `Source` | Project-relative JSON file describing the space, lights and probe placement. |
| `Chunks` | Project-relative pattern selecting chunk JSON files. Must match at least one file. |

The scene JSON supplies `space`, `fill`, `key`, `probePosition` and
`probeHalfExtents`. Chunks supply `space`, integer `cell` and `objects`.
Objects have a name, shape, size, position, Euler rotation and surface; shape
values are 0 for plane, 1 for box and 2 for sphere. Start by copying the
[sample room](../../../samples/reflections/assets/room.json) and
[chunk files](../../../samples/reflections/assets/chunks). There is no preset
that fills in omitted lighting or probe settings.

Capture settings are fixed: a 512×256 panorama, 32 m cells with origin
(-16, -16, -16), a 32 m ray range, and geometry from the probe's cell and its
immediate neighbors in the same space. Rays that miss geometry use the opaque
sky fill. Limits are 1 MiB per JSON file, 256 chunks and 4,096 objects total.
Unknown fields, invalid transforms/colors and geometry outside its cell fail
the build.

## How to use it

Put this pipeline in your [recipe](../getting-started.md#add-a-recipe):

```go
{
    Name: "reflections", Directory: "reflections",
    Input: []string{"assets/room.json", "assets/chunks/*.json"},
    Import: []assetbuild.Plugin{reflectionbuild.Scene(reflectionbuild.SceneOptions{
        Name: "Room", Source: "assets/room.json", Chunks: "assets/chunks/*.json",
    })},
    Process: []assetbuild.Plugin{assetbuild.ReflectionProbes()},
    Output: []assetbuild.Plugin{assetbuild.KTX2(), assetbuild.GoCatalog()},
},
```

`RoomScene()` returns the room's typed scene data for your application to create.
[ReflectionProbes](../processors/image-reflection-probes.md) turns the panorama into
six cube faces. [KTX2](../publishers/image-ktx2.md) and
[GoCatalog](../publishers/image-catalog.md) write the reflection texture and its Go
accessor.

Changing a distant chunk updates the generated room data without rebaking the
local panorama. Changes to nearby geometry, lights or the probe rebake it.
For the source format and runtime example, see the
[reflection build guide](../../../pkg/reflection/build/README.md).
