# Billboards

Three world-sized quads compare the available orientation modes:

- Left, red: spherical; faces the camera position and follows camera up.
- Middle, green: constrained to rotation around world Y. `BillboardOptions.Axis`
  can select another world-space axis.
- Right, blue: fixed; retains the entity's authored transform.

The retained panel labels the red, green and blue modes; **Reset view** restores
the comparison. Diagnostics start collapsed. Drag to orbit and use the wheel or pinch to zoom. Orbit above the quads to see
the difference between the spherical and axis-constrained modes. A fixed quad
can become edge-on; the billboards retain their world-unit dimensions.

```sh
mise exec -- go run ./cmd/engine run web ./samples/billboards
```

Billboard updates change instance transforms, without rebuilding geometry or
uploading its vertex/index buffers. Unchanged camera/object settings reuse their
cached transform. The same components and behavior work on native and browser
targets.

`main.go` owns startup and cleanup; `scene.go` creates the camera, lights and
comparison geometry. See the [shared sample conventions](../README.md).
