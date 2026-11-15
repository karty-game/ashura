# Application API

Import `github.com/karty-game/karty-webgpu/pkg/engine` for the application, world,
entities and shared math values. Domain packages such as `camera`, `mesh`,
`material` and `light` supply ordinary components.

## First scene

```go
package main

import (
    "errors"
    "log"
    "time"

    "github.com/karty-game/karty-webgpu/pkg/camera"
    "github.com/karty-game/karty-webgpu/pkg/engine"
    "github.com/karty-game/karty-webgpu/pkg/engine/features"
    "github.com/karty-game/karty-webgpu/pkg/light"
    "github.com/karty-game/karty-webgpu/pkg/material"
    "github.com/karty-game/karty-webgpu/pkg/mesh"
)

func main() {
    if err := run(); err != nil {
        log.Print(err)
    }
}

func run() (err error) {
    app := engine.NewApp(engine.AppConfig{
        Title: "First scene", Width: 1280, Height: 720, CanvasID: "game",
    })
    defer func() { err = errors.Join(err, app.Close()) }()
    if err := app.Register(features.Standard()); err != nil {
        return err
    }

    world := engine.NewWorld()
    projection, controls := camera.Orbit()
    view := world.Add(engine.DefaultTransform(), projection, controls)
    if err := world.SetActiveCamera(view); err != nil {
        return err
    }
    world.Add(light.DefaultHemisphere("sky"))

    surface := material.DefaultStandard()
    surface.Color = engine.ColorOrange()
    surface.Roughness = 0.6
    box := world.Add(mesh.NewBox("box", mesh.BoxOptions{}),
        engine.DefaultTransform(), surface)

    stop := app.OnUpdate(func(dt time.Duration) {
        world.Transform(box).Rotation.Y += float32(dt.Seconds())
    })
    defer stop()

    return app.Run(world)
}
```

Register GPU features explicitly before Run. `features.Standard`, `Source`,
`Impostors`, `Sprites`, `Compute` and `PointShadows` select implementations;
there is no implicit full renderer. Register the features needed by every mode
the scene can select. Duplicate/invalid batches fail without partial changes,
and missing features return `engine.ErrFeatureMissing` rather than a fallback.
Point shadows require a Standard or Source renderer. UI attaches separately
through `uiapp.Attach`, retaining its View and input/resource ownership.
`World.Add` and ordinary component constructors remain unchanged.

`App.ViewportSize` reads framebuffer dimensions without creating Screen2D.

This example is compiled in [api_examples_test.go](api_examples_test.go). Running
it opens a native window, or the named canvas when built for browser/WASM.
The [samples](../../samples/README.md) include the browser build and host setup.

## Setup, updates and errors

`NewApp` creates an owner; `Run` opens its backend. Declare compute buffers,
pipelines and bindings before `Run`. Use `RunContext` for cancellation from
another goroutine. `Run` closes the App when its active backend ends; the deferred
`Close` also handles failures before `Run`. Closing is idempotent.

`OnUpdate` is convenient for ordinary state changes. `OnUpdateError` accepts
`func(time.Duration) error`; returning an error stops the frame and reaches
`Run`. `OnCompute` accepts `func(*engine.Frame) error` and records GPU work after
CPU updates and before rendering. Preserve causes with `%w` when adding context.
Handle recoverable gameplay errors locally rather than stopping the App.

Each registration returns an idempotent cancellation function. Keep it when an
update belongs to a shorter scene lifetime:

```go
stop := app.OnUpdateError(updateScene)
if err := root.Own(stop); err != nil {
    stop()
    return err
}
```

Registration and cancellation run on the App loop goroutine. Cancellation is
immediate; new callbacks registered during their own phase start next time.
Cancel compute work before unloading the resources it uses. `App.Close` also
releases all update and compute callbacks.

Use `errors.Is(err, engine.ErrInUse)` when unloading needs to wait for live users.
`ErrClosed`, `ErrOwner` and `ErrPhase` distinguish ended lifetimes, invalid resource
ownership and operations unavailable in the current phase. `ErrInvalidEntity`
and `ErrInvalidComponent` describe rejected ECS mutations. `model`, `impostor`
and `level` share the closed/in-use categories. Errors retain descriptive context;
do not match their strings.

## Component values and defaults

`World.Add` copies component values. Read and change the stored copy through
`engine.Get[T]`, `world.Transform` or typed queries; editing the original value
after attachment does not edit its stored copy. `World.Set` replaces a component
and rejects nil values. Reacquire component pointers after replacing/removing
their component or entity. Mutations belong to the App loop goroutine.

Start from `material.DefaultStandard`, `material.DefaultSource`,
`light.DefaultPoint`, `light.DefaultDirectional`, `light.DefaultHemisphere` or
`reflection.DefaultProbe`, then set fields before attaching. These values let
zero mean zero: `source.Intensity = 0` starts a light disabled. Existing options
constructors keep their earlier defaulting rules; see [API contract](../../spec/api.md).

For custom camera or light components implement `ProjectionProvider` or
`LightProvider`. Return their named descriptors and use public projection/light
kind constants. Point lights have explicit `Position`, `Range` and `Shadow`
fields. Descriptor methods read live component values; they must not prepare
assets or create resources. Existing tuple providers remain supported.

## UI and resources

Build detached UI trees with constructors and `With*` helpers, then check one
error from `ui.New`. Construction errors survive subsequent valid configuration.
`Node.Err` can inspect them earlier. Use `Set*` methods for mounted updates and
handle their immediate errors. `errors.As` exposes `*ui.ValidationError` details;
`errors.Is` still matches `ui.ErrBounds`. See the [UI guide](../ui/README.md).

Configure application UI before attaching optional diagnostics:

```go
panel, err := debug.Show(app)
if err != nil {
    return err
}
panel.Hide() // Optional: start with the retained reopen button.
```

The [debug panel](../debug/README.md) shares the View returned by
`uiapp.View(app)` and its WebGPU submission. Retained UI is an explicit optional
integration: import `pkg/ui/app` and call `uiapp.Attach(app, view)` before Run.
The core engine and platform drivers do not import the UI or text-shaping stack.
`View.AddOverlay` can attach other bounded nonmodal panels without replacing
the application's root. These overlays use the View's existing font and theme.

Removing an entity or model instance ends its presentation. Unloading its shared
resource is separate and can fail with `ErrInUse`. Retain complete-batch admission
and ownership checks; failed release must not partially destroy the resource.
Prepared resources stay GPU-ready: source parsing, resizing, mip generation,
geometry conversion and repacking belong to the build packages.
