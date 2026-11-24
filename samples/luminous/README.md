# Luminous

Compare clustered and all-lights rendering in a castle courtyard with up to 240
lights. Lantern motes drift over the cobblestones, and two spotlights and a point
light cast moving shadows as the ballista crosses the courtyard. Shadows from the
fixed pillars are baked into the floor.

Run this command from the repository root to open the browser preview.

```sh
mise exec -- go run ./cmd/engine run web ./samples/luminous
```

## Controls

| Action | Control |
| --- | --- |
| Orbit | Drag or swipe |
| Zoom | Scroll or pinch |
| Use clustered light culling | **1** |
| Evaluate every active light | **2** |
| Cycle light layouts | **3** (240 distributed, 96 distributed, 240 overlapping) |
| Toggle baked-only lighting | **B** |
| Pause or resume the lights and ballista | **H** |
| Toggle the debug panel | **Debug**, **Hide**, or **F3** |

Focus the scene before using keyboard shortcuts in the browser. Pause before
switching between **1** and **2** to compare the same frame.

## Code and assets

`scene.go` builds the courtyard. `lights.go` creates the shadow-casting lights,
handles the comparison controls and advances the light rig. `accents.go`
animates the lantern motes. `asset.config.go` prepares the models, the baked
floor lighting and the Source material textures.
[Dynamic lighting guide](../../pkg/engine/README.md#dynamic-lights) ·
[Model instance guide](../../pkg/model/README.md)

On Linux, these commands prepare the assets and start the native sample.
The native sample uses Ashura's display and WebGPU dependencies.

```sh
CGO_ENABLED=0 GOFLAGS=-tags=nofakecgo mise exec -- go run ./cmd/engine assets generate --project ./samples/luminous
CGO_ENABLED=0 GOFLAGS=-tags=nofakecgo mise exec -- go run ./samples/luminous
```

## Copyright annex

- Towers, arch, walls, banners, trees, ballista and palette by Kenney, CC0 1.0.
  The models retain their original geometry. The shared colormap is bundled
  separately, and unused tangents were removed for Ashura's static GLB profile.
  [Castle Kit](https://kenney.nl/assets/castle-kit) ·
  [Author's OpenGameArt upload](https://opengameart.org/content/castle-kit)
- Cobblestone albedo and OpenGL normal map by Rob Tuytel, CC0 1.0. The recipe
  generates mips and derives the floor's Source SSBump offline.
  [Cobblestone Floor 001](https://polyhaven.com/a/cobblestone_floor_001)
