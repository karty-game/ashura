# Collision and scene queries

The shared Go API supports static triangle meshes, nearest-hit ray and capsule
casts, box trigger overlaps, and a fixed-step walking controller. Use it from
game updates on Linux and in the browser. A collision world belongs to the app
loop and can follow the lifetime of a logical scene or independently loaded area.

## Prepare geometry

Put low-detail collision proxy GLBs in their own asset pipeline:

```go
assetbuild.Pipeline{
    Name:   "collision",
    Input:  []string{"collision/*.glb"},
    Import: []assetbuild.Plugin{modelbuild.PreparedGLB()},
    Output: []assetbuild.Plugin{collisionbuild.StaticModels()},
}
```

Here `modelbuild` is `pkg/model/build` and `collisionbuild` is
`pkg/collision/build`. Proxy GLBs follow the [static GLB profile](../model/README.md).
Bake placement into the proxy nodes. The build flattens those transforms and
emits a deterministic bounding volume hierarchy over double-sided world-space
triangles. Each source supports up to 262,144 input triangles. The
`collisionbuild.Boxes(name, bounds...)` publisher produces the same data for a
box blockout. Use the generated `colliders` package in game code. Its Go declarations
carry the cooked data into the executable and require no file load or index build.

## Use the queries

```go
const (
    solids collision.Layers = 1 << iota
    checkpoints
)

var queries collision.World
floorID := queries.AddMesh(colliders.CollisionFloor(), solids)
queries.AddTrigger(collision.Bounds{
    Min: engine.Vector3{X: 3, Z: -1},
    Max: engine.Vector3{X: 5, Y: 2, Z: 1},
}, checkpoints)

shape := collision.Capsule{Radius: .32, HalfSegment: .53}
player := collision.NewCharacter(shape, engine.Vector3{Y: 1})
var storage [8]collision.ID

app.OnUpdate(func(elapsed time.Duration) {
    player.Advance(&queries, elapsed, desiredHorizontalVelocity, solids)
    view.Position = player.ViewPosition()
    view.Position.Y += .67

    overlaps := queries.AppendTriggers(storage[:0], player.Position(), shape, checkpoints)
    updateCheckpoints(overlaps)
})
// Call queries.Remove(floorID) when the owning scene drops the collider.
```

`Raycast` and `CapsuleCast` accept a displacement. The returned hit fraction is
relative to that displacement. The point lies on the surface, and the normal
opposes the incoming motion. Equal hits across colliders favor insertion order.
A layer mask matches any shared bit. A zero query mask matches no layers.

The Y-up capsule has height `2*(HalfSegment + Radius)`. A capsule cast at a
resting contact can move parallel to or away from the surface. Spawn the
controller clear of surfaces. It applies gravity and slides at 60 steps per
second, with up to eight steps per update. `Position` is the simulation center.
`ViewPosition` interpolates the latest two positions. Surfaces within 45 degrees
of horizontal count as ground when their contact normals face up.

Trigger overlaps sample the current capsule position. Track consecutive results
to detect entry and exit. For streamed areas, add and remove their prepared
colliders with their gameplay lifetime. The v1 API covers static placement and
removal, discrete trigger overlap, and capsule walking. Automatic stair
stepping, overlap recovery, collider motion and rigid-body dynamics are outside
this version.

The [collision courtyard](../../samples/courtyard/) shows how to combine the
controller, a ray-picked switch, and trigger volumes for plates, relics and the exit.
