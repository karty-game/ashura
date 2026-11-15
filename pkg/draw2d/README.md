# Pixel-perfect sprites

`draw2d` renders retained sprites above the 3D world and below UI on the App's
existing WebGPU device. Native Linux and browser builds use the same shader.
There is no world-camera transform, depth test or lighting in this layer.

Prepare images through the [asset pipeline](build/README.md), then use its
generated catalog:

```go
images, err := assets.Load()
if err != nil {
    return err
}

sprite := app.Screen2D().NewSprite(images.Icon)
sprite.SetPosition(10, 10)
```

For a prepared 32×32 image this covers `[10,42) × [10,42)`: pixel indices 10
through 41 on each axis. `(0,0)` is the top-left framebuffer corner; X points
right and Y points down. One unit is one output pixel. Resizing changes the
available area without moving or scaling the sprite. World resolution changes
do not affect these coordinates.

## Transforms and appearance

```go
sprite.SetPosition(100, 60)
sprite.SetScale(2, 2)
sprite.SetPivot(16, 16)
sprite.SetRotation(math.Pi / 4) // Radians; clockwise on screen.
sprite.SetTint(1, 0.5, 0.5)   // Linear RGB multiplier.
sprite.SetOpacity(0.75)
sprite.SetOrder(5)
sprite.SetClip(80, 40, 100, 100)
```

Setters return the sprite, so chaining is optional:

```go
sprite := app.Screen2D().NewSprite(images.Icon).
    SetPosition(10, 10).
    SetOrder(5).
    SetOpacity(0.75)
```

Defaults are native image size, top-left pivot, unit scale, no rotation, white
tint, full opacity, visible and order zero. The transform subtracts the local
source-pixel pivot, scales, rotates, then translates. Position is where the pivot
lands. Negative scale flips an axis. `SetImage` changes source dimensions without
resetting the transform or explicitly chosen pivot.

Nearest sampling preserves source pixels. Integer translation/scale and aligned
quarter-turn rotations preserve the pixel grid. Fractional transforms and
arbitrary rotations are supported, with resampled pixel footprints.

Lower orders draw first; equal orders retain creation order, including after
image changes. Batching never changes that transparency order. RGB tint and
opacity are independent. Finite tint channels and opacity clamp to `[0,1]`.
Source RGB is sRGB and alpha is straight coverage; tint applies in linear RGB,
then output uses the attachment's transfer/blend convention. Browser unorm and
native sRGB attachments can blend translucent colors in different spaces,
following the existing presentation convention.

## Clipping and coordinates

`sprite.SetClip(x, y, width, height)` and `screen.SetClip(...)` take half-open
integer rectangles in framebuffer coordinates. Effective clipping intersects
viewport, screen clip and sprite clip. Rotation does not rotate a clip.
`ClearClip()` removes a clip; an empty clip draws nothing.

CSS/window coordinates may differ from framebuffer pixels. Convert a logical
pointer position using the current logical viewport size:

```go
screen := app.Screen2D()
x, y := screen.FromLogical(pointerX, pointerY, logicalWidth, logicalHeight)
sprite.SetPosition(x, y)
width, height := screen.Size() // Current drawable dimensions.
```

At DPR 2, a 32-framebuffer-pixel sprite spans 16 CSS pixels. UI keeps its existing
logical coordinates and input priority.

## Ownership and errors

The App creates its screen on first access and closes it with the App. An unused
layer has no sprite GPU resources. `NewSprite` and ordinary property setters
return no errors. Prepared asset admission and application execution report
resource/platform errors through their normal error results.

Images are immutable prepared page/region references. `NewImage` copies admitted
RGBA8 bytes once; `Region` shares the page without pixel copies. These are
resource admission operations, not source PNG/JPEG loaders. The generated
catalog calls them for build-prepared payloads. Source decoding, resizing,
packing and gutter generation stay offline.

`NewPreparedImage(width, height, format, blocks)` also admits one already encoded
BC1, BC7, ETC2RGB, ETC2RGBA or ASTC4x4 page. Compressed dimensions must be multiples
of four and the byte count must match the exact block layout. Formats require
their enabled WebGPU compression family; unsupported pages fail before uploading
the incoming page set. There is no runtime decoder or format fallback. Pages use
the existing linear GPU format and shader color conversion; compression does not
change the image's color interpretation. Memory limits charge prepared block
bytes, and regions retain their original pixel dimensions.

`Destroy()` is idempotent. Destroying one sprite keeps pages shared by other
live sprites, including hidden ones. Do not copy `Sprite` or `Screen` values;
retain their pointers. Mutations belong to the App update goroutine.

Programming errors panic at the call site before mutation: NaN/Inf, transforms
beyond the documented bound, invalid clip extents, unadmitted images, copied or
destroyed sprite handles, closed screens and exceeded limits. Zero opacity,
zero/negative scale, offscreen positions and empty clips are valid.

Bounds are 65,536 live sprites, 256 distinct pages, 128 MiB of distinct prepared
page pixels per screen, 4,096 pixels per image side and 16,384 pixels per
framebuffer side. Position/scale/pivot magnitudes are bounded to 16,777,216;
rotation must be finite. Limits apply before allocating retained resources.

## Retained work and sample

Unchanged frames skip sprite scans, packing and record uploads. Changes rewrite
affected 80-byte instance records. Ordering, image, clip and active-span changes
may rebuild bounded batch metadata. GPU commands and rasterization still run
each presented frame. `screen.Stats()` exposes live sprites/pages/image bytes and
cumulative packed-record/batch-build counts.

Adjacent sprites sharing a page and clip batch when their persistent slots are
contiguous. Explicit reordering can increase draw calls. Hidden live sprites
retain residency; pages release when their last sprite reference is removed.
First use may upload prepared pages, and creating the first visible sprites
during an update can compile the pipeline once. Create startup sprites before
`Run` to prepare their pipeline during backend setup.

See the [sprite sample](../../samples/sprites2d/README.md) for prepared standalone
and atlas images, transforms, order, tint, opacity, clipping and UI composition.
Tilemap compilation and prepared animation sequences remain future pipeline
extensions; the runtime does not generate tile geometry or pack animation art.

The [v1 decision](../../spec/decisions/0004-screen2d-v1.md) records the contract and
implementation boundaries. Exact checks and remaining platform limits are in
[validation](../../spec/validation.md).
