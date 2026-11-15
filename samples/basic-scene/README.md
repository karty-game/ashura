# Basic scene

A ground plane, warm-colored cube and sphere, hemisphere light, and controllable first-person camera. The app owns the frame loop; the sample does not create a platform window or call WebGPU directly.

Run from the repository root:

```sh
mise exec -- go run ./cmd/engine run web ./samples/basic-scene
```

The command compiles the sample to WASM and serves a temporary browser build on a free localhost port. Open the printed URL, or forward its port when working remotely. The browser must support WebGPU. Click the canvas to capture the pointer. Use WASD to move, Space/Shift to rise and descend, and the mouse to look around. Press Escape to release the pointer.

This sample has no text overlay, UI controls or debug panel. The scene stays
focused on rendering and camera movement; control instructions live here rather
than in the application. Other samples demonstrate retained UI and diagnostics.
It explicitly registers only `features.Standard()`. Camera viewport updates use
`App.ViewportSize` without creating a sprite layer.

`main.go` owns startup and cleanup; `scene.go` builds the scene, selects its camera
and adjusts its field of view when the viewport changes. See the [shared sample conventions](../README.md).
