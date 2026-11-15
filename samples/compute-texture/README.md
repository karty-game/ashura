# Compute texture

A WGSL compute shader generates animated colored rings directly into a GPU texture.
The ordinary standard material samples that texture on a cube, sphere and tiled
ground. No source image, image codec, CPU pixel generation or runtime mip generation
is involved.

```sh
mise exec -- go run ./cmd/engine run web ./samples/compute-texture
# Or find Compute texture in the complete gallery:
mise run samples
```

Open the printed URL on its automatically assigned localhost port. Drag to orbit,
use the wheel or pinch to zoom, and use two fingers to pan. Click/touch the canvas
for keyboard focus. **Animate texture** or **Space** pauses/resumes GPU generation; the last generated
texture remains visible while paused. Diagnostics start collapsed.

`main.go` creates/runs the App. `scene.go` creates resources and populates the scene;
`wgsl/pattern.wgsl` contains the complete shader and is embedded unchanged with
`go:embed`. The uniform and pipeline are created once before `Run`. The App compiles
the pipeline during startup, then each scheduled compute pass writes the shared
texture before the scene's render pass.

```go
kernel, err := compute.NewPipeline(app, compute.PipelineDesc{
    Program: shader.WGSL("wgsl/pattern.wgsl", patternWGSL),
    EntryPoint: "generate_pattern",
    Layout: shader.Layout{
        shader.UniformLayout(0, 16),
        shader.StorageTextureLayout(1),
    },
})
```

The sample packs one `float32` time value into a reused 16-byte uniform payload with
`encoding/binary`; it does not upload a Go struct's memory or infer WGSL padding.
Bindings explicitly connect slots 0 and 1 to the uniform and output texture.
`App.OnCompute` records a 32×32×1 dispatch for an 8×8 workgroup size, covering a
256×256 output. Resources and bindings stay reusable; only changed uniform bytes
are uploaded. Pausing skips both uniform writes and compute dispatches after the
initial output has been generated.

The output is one-mip linear `rgba8unorm`, suitable for storage writes and standard
material sampling. It has no CPU image payload and is shared by all three meshes.
This first compute API uses group zero, explicit byte buffers and portable storage
texture support. Typed WGSL packing, buffer readback and custom render pipelines
are separate milestones.

Resource creation belongs before `Run`; CPU buffer writes and dispatch scheduling
happen in callbacks. Unload a bind group before unloading resources or its pipeline.
`App.Close()` releases all remaining handles. No backend-specific WebGPU or
JavaScript object appears in scene code.
