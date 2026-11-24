# Collision courtyard

Collect three gold relics and race to the exit. Use the cyan switch to open
the copper gate. A violet pressure plate opens the timed gate for five seconds
after you step off. A second plate serves the return trip. The exit turns green
when you've collected all three relics.

Run this command from the repository root to open the browser preview.

```sh
mise exec -- go run ./cmd/engine run web ./samples/courtyard
```

## Controls

| Action | Keyboard and mouse | Touch |
| --- | --- | --- |
| Move | WASD | Drag **Move** |
| Look | Hold the left mouse button and drag | Drag the scene |
| Use the aimed switch | Space | Tap **Use** |
| Reset the current run | R or **Restart** | Tap **Restart** |
| Play the other route after finishing | R or **Next route** | Tap **Next route** |
| Toggle the debug panel | **Debug**, **Hide**, or **F3** | Tap **Debug** or **Hide** |

The clock starts when you move. Each completed run unlocks the other route.
Best times are kept for the session. The browser HUD shows the gate countdown,
and the native sample logs objectives and finish times. **Restart** and
**Next route** appear at the lower right. On narrow browser viewports,
opening the debug panel temporarily hides the HUD and reticle to make room.

## Code and assets

`scene.go` places the course and its colliders. `challenge.go` uses a capsule
for walking, a ray cast to aim at the switch, and overlap triggers for plates,
relics and the exit. `controls.go` uses the App's touch input for looking so
scrolling the debug panel leaves the camera still. `asset.config.go` prepares
collision meshes from the shared layout. [Collision API guide](../../pkg/collision/README.md)

On Linux, these commands prepare the assets and start the native sample.
The native sample uses Ashura's display and WebGPU dependencies.

```sh
CGO_ENABLED=0 GOFLAGS=-tags=nofakecgo mise exec -- go run ./cmd/engine assets generate --project ./samples/courtyard
CGO_ENABLED=0 GOFLAGS=-tags=nofakecgo mise exec -- go run ./samples/courtyard
```

If an older local build used a different output root, move its ignored
`samples/courtyard/generated` directory aside before preparing again.

## Copyright annex

The bricks, concrete, checker tiles, wood, panels, hazard stripes, arrow and
waypoint are by the Karty contributors. The recipe prepares mipmapped textures
offline and includes the MIT notice in the web distribution.
[Karty FPS kit](https://github.com/karty-game/karty-packages/tree/7da3077fa562c8274b17036a30e6ef175ee154bd/packages/fps-kit) ·
[MIT notice](assets/fps-kit/LICENSE.md)
