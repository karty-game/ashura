# Particles

Use the particlr editor to author effects and export version 13 `.prt` files.
Add `particle/build.Effects()` as the output plugin in the project's asset
recipe. The generated `assets.Load(name)` function accepts the source filename
without its extension. It reads the prepared atlas and simulation data.

Register `features.Particles()` and add the loaded effect before `Run`.
For a gameplay effect, call `app.AddWorldParticles(effect, origin, unitsPerPixel)`.
The origin is a 3D position and the scale maps each authoring pixel to a world
length. Individual particles face the active camera, and scene geometry occludes
them through depth testing. `SetWorldPosition` moves the emitter and its local
particles.

Imported layers use their authored XY motion by default. To give selected
layers depth, use `particle/build.Effects3D` in the asset recipe. Key overrides
by the effect's filename stem and the layer's exact `.prt` ID. A `Sphere` spawns
in a volume and sends particles outward. A `Cone` uses the Particlr cone's
direction, a starting disc radius, and a full spread angle in degrees.
`GroundPlane` maps the authored motion to world XZ. Radii and position offsets
use authored pixels. The [sample asset recipe](../../samples/particles/asset.config.go)
shows all three shapes and a rising emitter for the firework trail.

Use `app.AddParticles(effect, x, y)` for a screen-space effect with a
framebuffer-pixel origin. Both placements support `Stop` and `Restart` on the app loop.
They reserve GPU slots before the first frame and one-shot effects stop
drawing after their last particle expires. Screen placement uses the simulation's XY
coordinates.

The current version supports local 2D emitters, normal and additive blend modes,
built-in textures, embedded PNGs up to 64 pixels per side, motion and
lifetime curves, and velocity-aligned stretch. Export packs the PNGs into
the prepared atlas.
The build bakes prewarm into the asset so an effect can start with live particles.
The build reports an
error for effects that require unsupported features such as flipbooks,
trails, collision, or sub-emitters. The live particles update in
WGSL and render on the same WebGPU device as the scene.

The [Particles sample](../../samples/particles/README.md) shows the asset recipe
and demonstrates depth-tested playback with 29 adapted CC0 presets. The
[preset catalog](../../samples/particles/presets.md) links all 71 editable
originals and records current importer compatibility.
