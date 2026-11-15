# Offline primitive-scene reflections

`reflectionbuild.Scene` imports a bounded primitive scene, generates typed scene
literals and bakes its diffuse panorama. It is a shared build module; applications
consume generated declarations and prepared textures without importing this package.

```go
Pipelines: []assetbuild.Pipeline{{
    Name: "reflections",
    Input: []string{"assets/room.json", "assets/chunks/*.json"},
    Import: []assetbuild.Plugin{reflectionbuild.Scene(reflectionbuild.SceneOptions{
        Name: "Room", Source: "assets/room.json", Chunks: "assets/chunks/*.json",
    })},
    Process: []assetbuild.Plugin{assetbuild.ReflectionProbes()},
    Output: []assetbuild.Plugin{assetbuild.KTX2(), assetbuild.GoCatalog()},
}},
```

Use `github.com/karty-game/karty-webgpu/pkg/reflection/build`. `Name` is an
exported Go identifier. The example emits `RoomScene()` (returning
`pkg/reflection/scene.Scene`) and the regular `RoomReflection()` texture catalog.
Source and chunk paths go through the asset pipeline's confined tracked reads.
The two source files in the [reflection sample](../../../samples/reflections/assets/)
show the JSON structure. Unknown fields, invalid colors/transforms and geometry
outside its declared cell are rejected before baking.

The settings record contains `space`, hemisphere/directional lighting and probe
position/bounds. Chunk records contain `space`, integer `cell` and `objects`.
Object shapes are centered plane (0, size X/Z), box (1, size X/Y/Z) and sphere
(2, size X diameter); positions and Euler rotations match engine transforms.
Runtime consumers build their ordinary meshes from the generated typed records.
The module emits data, not application setup or a renderer extension.

One probe uses its 32 m cell and immediate neighbors, including diagonals, in the
same space. Grid origin is [-16,-16,-16]; rays stop at 32 m and misses use opaque
sky fill. A distant chunk edit updates the complete typed scene without rebaking
the panorama. Neighbor membership, nearby geometry, probe placement or shared
lighting edits invalidate the local bake. Recipe executable identity includes
library algorithm changes; no project-local Go-source glob is required.

The deterministic 512×256 panorama uses ideal primitive intersections and diffuse
lighting. It excludes recursive reflections, specular highlights and arbitrary
model capture. ReflectionProbes projects and filters its 128px cube faces offline.
Admission retains 1 MiB per JSON file, 256 chunks and 4,096 objects, with bounded
coordinates. There is no runtime JSON parsing, source decoding or repacking.
