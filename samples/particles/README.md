# Particles

Explore 29 GPU particle effects in world space. Raise the wall to see opaque
geometry hide the particles behind it. The **Ion Plume** runs continuously while
you browse.

Run this command from the repository root to open the browser preview.

```sh
mise exec -- go run ./cmd/engine run web ./samples/particles
```

## Controls

| Action | Control |
| --- | --- |
| Preview a preset | Choose a **Collection**, then an **Effect** |
| Play it again | **Replay effect** |
| Check occlusion | Toggle **Wall** |
| Orbit | Drag or swipe |
| Zoom | Scroll or pinch |
| Toggle the debug panel | **Debug**, **Hide**, or **F3** |

## Code and assets

`gallery.go` loads prepared effects, places them in world units and handles
playback. `presets.go` groups the effects for the picker. `asset.config.go` sets
each layer's 3D emission shape during preparation. The [preset catalog](presets.md)
links the editable sources and describes the featured effects.
[Particle API guide](../../pkg/particle/README.md)

On Linux, these commands prepare the assets and start the native sample.
The native sample uses Ashura's display and WebGPU dependencies.

```sh
CGO_ENABLED=0 GOFLAGS=-tags=nofakecgo mise exec -- go run ./cmd/engine assets generate --project ./samples/particles
CGO_ENABLED=0 GOFLAGS=-tags=nofakecgo mise exec -- go run ./samples/particles
```

## Copyright annex

The effects adapt CC0 Particlr presets for the 3D stage. The source library in
`assets/particlr/` contains all 71 originals from `dist/assets/presets` in
`@particlr/mcp` 0.3.1. The package archive SHA-256 is
`e54a68a26c3fbc56fb66074e8335cd4a212c5893a4d5ef07dca0a5e6f09e919d`.
The importer tests use the original Explosion and Smoke files from this library.
[Particlr source package](https://www.npmjs.com/package/@particlr/mcp/v/0.3.1) ·
[Contact sheet](https://particlr.com/presets-contact-sheet.png) ·
[CC0 1.0](https://creativecommons.org/publicdomain/zero/1.0/)
