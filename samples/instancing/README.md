# GPU instancing

Creates 5,000 cube instances from one source mesh and animates their height and
rotation into a traveling two-dimensional wave. The instances share the cube
geometry and material and are submitted as a GPU instanced draw.

Run from the repository root:

```sh
mise exec -- go run ./cmd/engine run web ./samples/instancing
```

Use **Animate instances** or **T** to pause/resume the wave; camera controls
remain available. Diagnostics start collapsed. The panel explains the instance
count and shared mesh.

The sample uses the first-person camera controls. Click the canvas to capture
the pointer, move with WASD, and look around with the mouse. Press Escape to
release the pointer.

`main.go` owns startup and cleanup; `scene.go` creates the shared geometry,
instances and update callback. See the [shared sample conventions](../README.md).
