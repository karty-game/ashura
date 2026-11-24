# Projected decals

Project an albedo image onto opted-in Standard or Source meshes. Register
features.Decals alongside the material features before running the App.

    if err := app.Register(features.Standard(), features.Decals()); err != nil {
        return err
    }
    const walls = 1
    if err := world.Set(wall, decal.Receiver{Layers: walls}); err != nil {
        return err
    }

    projector := decal.DefaultProjector(decal.Image{Texture: atlas})
    projector.Position, projector.Normal = hit.Point, hit.Normal
    projector.Size = engine.Vector3{X: .4, Y: .4, Z: .12}
    projector.ReceiverMask = walls
    impact := world.Add(projector)

The box is centered on Position and projects along the inverse of Normal.
Size gives its width, height and total depth. Angle rotates around Normal
in radians. The shader blends texture alpha times Tint.A before lighting
and preserves the receiver's coverage. Larger Order values blend later.
Equal orders use entity creation order.

Use world.Set or engine.Get to edit a live projector and world.Remove(impact)
to remove it. Each world supports up to decal.MaxProjectors live components.
A zero UV rectangle selects the whole texture. Supply normalized, padded
atlas regions to select individual images.

## Authored marks

Use the build plugins to bake permanent marks into an albedo or prepare
toggleable marks as immutable projector records. Mount a prepared batch with
world.MountDecals and its shared atlas. Call placement.SetEnabled with the
authored index to toggle a mark. Call placement.Remove when the batch leaves
the world. Level chunk presentations own and remove their attachments.
Use world.MountDisabledDecals to reserve a batch with all records hidden until
the game enables them.
See the [build guide](build/README.md) and [level presentation guide](../level/README.md).

Live and authored projectors share one immutable App-owned sRGB alpha atlas.
Authored batches use a separate pool of 8,192 slots, reserved in groups of 32
for each batch. The GPU bins authored candidates into a 16×16 screen grid.
Tiles with up to 64 candidates use a compact list. Denser tiles traverse their
complete candidate mask. In both cases, authored candidates blend by Order,
with slot order breaking ties. Live marks blend over the authored layer.

The [Decals sample](../../samples/decals/README.md) shows how to prepare an
atlas and place a mark from a collision raycast.
