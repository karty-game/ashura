# Debug panel

Attach the optional retained UI panel before running the App. To start with
the small **Debug · F3** button, call `panel.Hide()` as the samples do.

```go
import "github.com/karty-game/ashura/pkg/debug"

panel, err := debug.Show(app)
if err != nil {
    return err
}
panel.Hide()
return app.Run(scene)
```

The panel uses the same WebGPU UI library, input routing and submission as the
application on browser and native targets. It creates no HTML nodes, CSS,
browser event listeners, polling timer or separate animation loop. If the App
already has a UI View, the panel shares it without replacing its root, font or
theme. Configure that application View with `App.SetUI` before `debug.Show`;
calling `SetUI` afterward replaces the entire surface, including its overlays.
Otherwise the panel creates an empty View using the offline-prepared default
font. Font/resource admission occurs once during setup; no glyph rasterization
or atlas packing occurs at runtime.

F3 toggles the panel when the sample has keyboard focus. Escape hides the panel
unless an application modal, popup, or focused text input handles it. The
retained **Hide** and **Debug** buttons support pointer, touch, and keyboard
activation. Showing the panel makes the
cursor available; hiding it permits normal canvas pointer lock again. The
header stays above a separately scrollable metrics region, which can receive
keyboard focus for scrolling. Controls have 44px minimum heights. The top-right
overlay adapts to the viewport and uses at most 70% of its height, leaving the
scene accessible. It is nonmodal; pointer input outside its bounds and normal
application controls continue to work.

`debug.Show` now returns an error for closed Apps, font/UI admission and resource
bounds instead of silently dropping the panel. Runtime metric-text errors remain
available through `Panel.Err()` and propagate through the App update loop to
`Run`. Panel methods and snapshots belong to the App loop goroutine; they do not
provide concurrent reads.

The panel displays frame cadence, CPU phases, rendering counters, compute
dispatches, Go memory and allocation rates, approximate JS heap, and
loaded/resident texture counts. Rendering is separated into **Scene 3D** mesh
instances/triangles, **2D sprites**, and **UI**, with the panel's own glyph and
panel quads included in UI. Total draw calls cover all three categories. A UI
quad is an instanced rectangle made from two triangles, not a scene object.

The UI count changes with visible text and clipping. The observer's flat
`FrameStats.DrawCalls`, `Instances` and `Triangles` report combined totals.
`Scene`, `Sprites` and `UI` expose separate counts.

Loaded/resident counters describe the engine-owned sampled/storage texture
cache, including fallback maps. Resident bytes estimate
its mip payload; UI/sprite atlas pages, presentation/depth/MSAA attachments and
driver overhead are excluded.

Memory and metric text refresh at most four times per second while visible.
Other observer calls only accumulate value counters; hidden panels suspend
collection. Retained labels and unchanged text are reused. Go allocation rates
are process-wide sampled counter differences divided by observed frames or wall
time, including engine, application, runtime and debug-panel allocations. The
panel reports its previous sampling/formatting/text-update duration separately;
UI layout and rendering occur in the ordinary subsequent App frame.


`Go heap in use ≈` counts heap-object bytes, including live objects and dead objects awaiting collection;
growth between samples alone does not establish retained growth or a leak.
`Go live after GC` reports the bytes marked live by the most recently completed
garbage collection, so it changes at GC boundaries rather than every refresh.
The sampled allocation counters may update in batches when the runtime flushes
allocation-span statistics, so a short window can show zero despite small
allocations. Unsupported runtime counters display `unavailable`, including the
dummy `runtime/metrics` implementation in pinned TinyGo 0.44.0. No full heap scan
or forced collection is introduced to replace missing counters.
The per-frame allocation rate includes the standard Go/WASM callback and
WebGPU object-reference bridge even when application updates and instance uploads
allocate nothing. Changing numeric labels can also allocate during the periodic
refresh. Their font shaping uses retained scratch storage and bounded caches.

`Frame interval` measures loop cadence. `Engine CPU` excludes time between
frames and observer sampling. The measured interval starts immediately before
application update; platform input polling and canvas resize checks are outside
that CPU total. Phase times are CPU wall time; material preparation includes
texture/uniform uploads, and native submission/presentation may wait. These are
not GPU execution or physical-display measurements. GPU elapsed time and actual
JS allocation counters are explicitly unavailable. On browsers exposing
`performance.memory`, JS heap is approximate live memory and its delta is net
heap change, which can be negative after collection. Native builds mark JS heap
unavailable.

`debug.Show` replaces the App's existing frame observer. Observer replacement or
App shutdown closes the previous panel, removing its overlay nodes, global UI
shortcuts and error callback. Closing a panel preserves application-owned UI.
For custom presentation or telemetry, use the observer directly:

```go
app.SetFrameObserver(engine.FrameObserverFunc(func(stats engine.FrameStats) {
    // Consume stats without blocking the loop.
}))
```
