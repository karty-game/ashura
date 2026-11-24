# WebGPU UI

Retained Go controls built from panels, shaped text and prepared images. UI core
code has no DOM, JavaScript, window-system or GPU-driver imports. Ashura's App
owns the renderer, event pump and resource lifetime.

```go
face, err := defaultfont.Load()
if err != nil {
    return err
}

name := ui.NewInput("Name", "Explorer", "Your name", func(change ui.Change) {
    fmt.Println(change.Text)
})
root := ui.Column(
    ui.NewText("Settings"),
    name,
    ui.NewSwitch("Sound", true, nil),
    ui.NewButton("Apply", func() { fmt.Println(name.Text()) }),
).WithStyle(ui.Style{Padding: 24, Gap: 12, Scroll: true})

view, err := ui.New(root, face)
if err != nil {
    return err
}
if err := uiapp.Attach(app, view); err != nil {
    return err
}
```

Import `ui`, `ui/app` (package `uiapp`) and optional `ui/fonts/defaultfont` from
`github.com/karty-game/ashura/pkg/`. Attach before `App.Run` and retrieve
the attached concrete view with `uiapp.View(app)`. Passing nil to
`uiapp.Attach` detaches it before Run. The engine and platform drivers do not
import retained UI, fonts or text shaping; apps that never attach UI leave those
implementations out of their dependency graph. All View/Node/Font
operations belong to one UI/platform owner thread. A Font's shaping state is
mutable; use separate loaded faces across independently scheduled views.

### Nonmodal overlays and global shortcuts

`View.AddOverlay` admits a detached subtree into the same View, sharing its
prepared font, theme, image ownership, input and retained draw storage:

```go
metrics := ui.Column(ui.NewText("Ready")).WithStyle(ui.Style{Grow: 1, Scroll: true})
if err := metrics.SetFocusable(true); err != nil {
    return err
}
var panel *ui.Node
panel = ui.Column(ui.NewButton("Hide", func() { panel.SetHidden(true) }), metrics)
remove, err := view.AddOverlay(panel, ui.OverlayOptions{
    Anchor: ui.OverlayTopRight, Width: 360, MaxHeightRatio: 0.7, Margin: 12,
    Cursor: true,
})
if err != nil {
    return err
}
defer remove()
```

The four `OverlayTopRight`, `OverlayTopLeft`, `OverlayBottomRight` and
`OverlayBottomLeft` anchors use viewport logical pixels. Width zero uses the
available width or the root's narrower `Style.Width`; height is intrinsic up to
the requested fraction of available viewport height. Zero `MaxHeightRatio`
means the full available height. A capped overlay root fits flexible `Grow`
children into that height, so a fixed header and scrollable body remain usable
on smaller viewports. This fitting does not change normal or modal flow.

Overlays paint after ordinary content, in registration order, and below modal
scopes. They consume pointer/wheel events inside their clipped bounds, including
noninteractive panel chrome, while unrelated world/UI input stays nonmodal.
`Cursor` makes `PreventsPointerLock` true while that overlay root is visible;
the App releases pointer lock without capturing input outside the panel. Hide
the root with `SetHidden` to suspend both its painting and cursor request.
The removal function and root `Remove` are idempotent; removal clears focus,
capture, popup, resource references and stale IDs. Existing View node/image
bounds also cover overlays. Unchanged overlays reuse their retained frame.

`View.OnPreview(func(*RoutedEvent))` observes input before node routing even with
no focused node. It returns an idempotent cancellation function. `Current` is nil
for these View-wide callbacks; `Target` still identifies the current event
target where present. `PreventDefault` consumes the platform event and skips its
UI default, while `StopPropagation` stops later callbacks/node routing without
skipping a default by itself. A registration made during dispatch starts on the
next event, and cancellation immediately suppresses a pending callback.
Use this for shortcuts such as a diagnostic panel's F3/Escape controls. Panel
`SetFocusable(true)` adds a scroll region to Tab navigation and enables its
existing Arrow/Page/Home/End scrolling; controls keep automatic focus behavior.
Platform wrappers can call `DispatchPreview` when another native/HTML control
owns focus: it runs only these global handlers, without layout, node routing,
default keyboard actions or changes to UI focus. Ordinary `Dispatch` continues
to route input for the View's own canvas/surface.

## Controls and styling

Available controls include text, panels, prepared images, buttons, checkboxes,
binary switches, toggle buttons, stepped sliders, select-only dropdowns and
single-line inputs. Tabs, separators, tooltips, persistent alerts, determinate
progress, bounded pagination and read-only charts compose with the same retained nodes.

`NewButton` uses `SecondaryButton` styling. `SetButtonVariant(PrimaryButton)`
marks the main action; `QuietButton` is for lower-priority utility actions.
`Theme.PrimaryButton` and `Theme.SecondaryButton` are `ButtonPalette` values with
fill, hover, pressed, foreground and border colors, so action styling remains
distinct from input surfaces. Focus, disabled state and activation behavior are
shared across variants. Explicit node colors still override the palette.

### Large collections

Use `NewVirtualList` for long result tables, inventories and debug logs. Ordinary
`Column` panels retain every child; they are intended for bounded UI composition.
A virtual list is a scrollable Panel with fixed-height rows and a reusable pool
sized to the visible clipped viewport plus overscan. Ten thousand logical items
do not create ten thousand controls. Application data stays outside the row nodes.

```go
rows := ui.NewVirtualList(ui.VirtualListOptions{
    Count: len(results), RowHeight: 44, Overscan: 2,
    Create: func() *ui.Node { return ui.NewButton("", nil) },
    Bind: func(row *ui.Node, index int) error {
        return row.SetText(results[index].Name)
    },
}).WithStyle(ui.Style{Height: 320, Padding: 8})

// After changing data, invalidate only its logical range.
if err := rows.RefreshVirtualRows(changedIndex, 1); err != nil {
    return err
}
// A full refresh also visits only the mounted pool.
if err := rows.RefreshVirtualRows(0, len(results)); err != nil {
    return err
}
```

`SetVirtualCount` updates count/clamps the offset; `ScrollToRow` jumps directly
without visiting intermediate items. Offscreen invalidation causes no frame work;
rows bind to current application data when they enter the window. Hidden/clipped
lists also defer count changes and jumps until they are visible again. Fixed row height
includes spacing; use row padding for gaps. Zero height defaults to a 240-pixel
viewport, and `Grow` can fill remaining space. Zero overscan defaults to two rows
on each side. `VirtualListStats` reports logical count, the current half-open
range including overscan, mounted pool size and cumulative binding calls.

`Create` constructs a reusable row subtree. `Bind` updates existing nodes and
must be cheap: do not fetch/decode assets, enumerate the whole dataset, add/remove
children, dispatch events or recursively change the list. Callback errors propagate
through `View.Frame`; a partially completed bind is not rolled back. Bind every
item-dependent value, and have callbacks read the row's current bound index.
Editable values and selection belong in the application model, not in a recycled
node. Reassigning a row invalidates its old IDs and interaction/editing state;
focus moves to the list if that item leaves the mounted window. Items that remain
in the window retain identity/focus when the pool grows. Touch-scroll capture
continues on the list across recycling. Cancellation/composition notifications
run with the old bound identity before reassignment; callback-driven removal
restarts placement safely, and row-owned modal scopes close when retired. Arrow/Page/Home/End keys scroll a focused
list; Tab visits currently mounted controls. This is not a full logical-item
selection or offscreen keyboard traversal controller. Semantic row roots expose
`PosInSet`/`SetSize`; no platform accessibility bridge is added.

Limits are 1,000,000 logical items, 16,777,216 logical pixels of total extent,
256 mounted row roots and 16 overscan rows per side. The normal View node/depth,
record and image bounds also apply to the pool. A viewport requiring more than
256 rows returns `ErrBounds`. Pool capacity follows the largest visible window
until the list is discarded. Variable-height rows, arbitrary data sorting and
model construction remain application work. The [Performance playground](../../samples/ui/)
exercises 10,000/100,000 rows, direct jumps and data updates.

### Source images and build integration

Author icons, frames and backgrounds as separate PNG files. An
[asset recipe](../assetbuild/README.md#ui-pngs-and-automatic-atlases) composes
`assetbuild.Images()`, `assetbuild.Atlas(...)` and `uibuild.Images()` from
`github.com/karty-game/ashura/pkg/ui/build`. The host plugin writes prepared
RGBA pages and a typed generated catalog. Load that catalog once and pass its
named images directly to controls; all regions on a page share the same resource.
Neither application UV coordinates nor a handwritten sample atlas are needed.
PNG decoding, packing and gutter extrusion run during the build; the generated
loader adds no runtime image decoding. The [playground recipe](../../samples/ui/asset.config.go) shows
this workflow for buttons and panels together.

### Panel images and 9-slice frames

`SetPanelBackground` decorates a Panel using an immutable `ImageAsset`, including
atlas regions. Plain images support `StretchImage`, `ContainImage` and
`CoverImage`: stretch fills the panel, contain preserves proportions with
letterboxing, and cover fills it with a centered crop. `Tint` defaults to white;
`PanelBackgroundPart` can override the image tint and rounded mask radius.

For a frame, specify the four fixed source borders:

```go
panel := ui.Column(title, content).
    WithStyle(ui.Style{Padding: 24, Height: 240}).
    WithPanelBackground(ui.PanelBackground{
        Image: frameArt,
        Slice: &ui.NineSlice{
            Left: 32, Top: 32, Right: 32, Bottom: 32,
            Scale: 0.5,
        },
    })
// ui.New reports any invalid construction configuration when mounting the tree.
```

Insets are integer source pixels relative to the image/region; opposing borders
must leave a positive center. Corners retain their size while edges and center
stretch. `Scale` converts source pixels to logical UI pixels (zero defaults to
one), so 32-pixel borders at scale 0.5 produce 16-pixel UI borders. It accepts
0–64; when a target is smaller than its opposing borders, those borders compress
proportionally to fit without overlap. `BorderOnly` omits the center, allowing
the panel's own fill or lower layers to show through. Sliced backgrounds use
`StretchImage`; tile/repeat modes are not implemented.

Backgrounds do not change content padding, intrinsic layout, hit testing or
semantic/focus structure. Set `Style.Padding` separately. Painting order is
panel fill, image/frame, optional style border, scrollbar, then children.
Scroll backgrounds stay attached to the viewport. The rounded mask applies to
the whole panel, and source sampling is constrained to the image/slice region
to avoid neighboring atlas pixels bleeding into the border.

Descriptors/getters copy slice metadata. Asset pages remain shared and count
once toward the View image budget. Replacements validate atomically and repaint
only the panel; unchanged descriptors are no-ops. Clear with
`SetPanelBackground(ui.PanelBackground{})`. Up to nine contiguous image quads
reuse one texture batch. Source pixels and slicing metadata are prepared
offline; rendering does not decode, repack or resize image payloads.
The [Panels playground](../../samples/ui/) starts with a landscape behind live
controls inside a resizable gold/steel frame. It composes a border-only outer
panel with an inset image-background panel, keeping illustration and frame
independent; Show background and Show frame control each layer separately. It also shows tint, safe small
panels, fitting modes and framed scrolling.

### Interaction motion

Buttons, toggle buttons and inputs use Material-inspired interaction feedback
while retaining the existing palette. Hover/press/selection colors and button
corner shape transition smoothly from their current displayed state, including
when an interaction reverses mid-animation. Pressing a button creates a bounded
ripple at the pointer; Enter/Space use a centered pulse. Ripples stay inside the
rounded surface, render behind text/artwork and fade on release. Activation
callbacks keep their normal timing.

Inputs gain an animated focus border and a center-expanding active indicator.
Labels, text, caret geometry and hit regions stay fixed. Keyboard focus outlines
are visible immediately. Existing external field labels remain visible; this
does not change the text-editing or composition contract.

Default hover/press colors stay close to each control's resting color. Quiet
buttons use faint foreground overlays (4% hover, 7% press) rather than an opaque
filled surface; ripples are restrained. Focus and selected-state colors remain
explicit so transient feedback does not resemble a persistent selection.

`Theme.Motion` contains a `MotionTheme`: hover 120 ms, press 80 ms, focus 160 ms,
release 160 ms, ripple expansion 320 ms and ripple fade 180 ms by default. Each
duration accepts 0–2 seconds; zero makes that transition immediate, and zero
ripple expansion suppresses the ripple. For example:

```go
theme := view.Theme()
theme.Motion.HoverDuration = 100 * time.Millisecond
theme.Motion.RippleFadeDuration = 140 * time.Millisecond
if err := view.SetTheme(theme); err != nil {
    return err
}
```

`Theme.Motion.Disabled` disables these interaction animations. The existing
`View.SetReducedMotion(true)` also snaps transitions and suppresses ripples;
the browser wrapper forwards the system preference. `RipplePart.Fill` sets the
ripple color/max opacity (6% of foreground alpha by default), and
`ActiveIndicatorPart` styles the input indicator. Disabled/read-only buttons,
hidden or detached controls and canceled touch scroll presses do not retain
active ripples.

Only active controls are scheduled. Transitions return to retained idle frames;
a fully expanded held ripple also stops changing paint until release. There
are no per-control timers, extra textures or layout animations. The
[Motion playground](../../samples/ui/) demonstrates pointer/keyboard feedback,
input validation and the application-level effects switch. The design draws
on [Material ripple guidance](https://material-web.dev/components/ripple/) and
[text-field states](https://material-web.dev/components/text-field/); it uses
the existing WebGPU renderer and prepared font.

### Button icons and artwork

`SetButtonVisual` adds an icon or prepared image to `NewButton` and `NewToggle`.
The existing button text remains its visible label. `LeadingContent` places the
visual before the label; `TrailingContent` places it after. Empty button text
centers the visual for toolbar actions and inventory slots. These remain one
control, with the same pointer capture, keyboard activation, focus and toggle
state as a text button. Supply `SetLabel` with an action/item name for visual-only
controls; a tooltip can add supplementary help.

```go
buy := ui.NewButton("Buy", purchase).
    WithButtonVisual(ui.ButtonVisual{Image: coin, Side: ui.LeadingContent, Size: 24})

increase := ui.NewButton("", increaseQuantity).
    WithLabel("Increase quantity", "").
    WithButtonVisual(ui.ButtonVisual{Icon: "+", Size: 20})
```

`ButtonVisual.Icon` uses prepared-font text, suitable for symbols such as `+`,
`−` and `?`. Pictograms outside the font's glyph coverage use image artwork.
`Image` references an immutable `ImageAsset` created with `NewImageAsset` from
GPU-ready straight-alpha RGBA8 pixels. `ImageAsset.Region` selects an integer
pixel rectangle from that asset, so buttons can share an offline-prepared atlas.
The resource is copied once on admission and shared regions reuse its texture;
there is no runtime image decoding, rasterization or atlas packing. Images fit
their visual box while preserving aspect ratio. `Size` defaults to 20 logical
pixels and `Gap` to 8; clear the visual with `ButtonVisual{}`. `IconPart.Fill`
overrides symbol color and `ImagePart.Fill` applies an image tint; images retain
their original colors by default and dim when disabled.

Tabs accept the same `Tab.Visual` and an optional `AccessibleLabel` for
visual-only labels. The [Game UI playground](../../samples/ui/) demonstrates
leading/trailing symbols and badges, named symbol-only actions, image inventory
slots, and image/icon tabs.

### Charts

`NewChart` supplies line, grouped bar, pie and radar charts as retained nodes for
results, stats and debug panels. Cartesian charts draw numeric axes and category
labels; pie charts show category/value legends; radar charts compare named
attributes with filled outlines. Series colors, palettes, grid/axis/label parts
and explicit ranges share the existing theme and typed styling.

```go
chart := ui.NewChart(ui.LineChart, "Frame time in milliseconds", ui.ChartData{
    Labels: []string{"1", "2", "3", "4"},
    Series: []ui.ChartSeries{
        {Name: "CPU", Values: []float64{10, 12, 11, 17}},
        {Name: "GPU", Values: []float64{7, 8, 9, 12}},
    },
}).WithStyle(ui.Style{Height: 240}).
    WithChartOptions(ui.ChartOptions{Range: &ui.ChartRange{Min: 0, Max: 33}})
```

Use `LineChart`, `BarChart`, `PieChart` or `RadarChart`. `ChartData.Labels` names
the samples/categories/attributes, and each series supplies one value per label.
Line samples have equal horizontal spacing. Auto ranges include zero; explicit
bar ranges must also contain zero. Pie charts require one nonnegative series;
radar values are nonnegative and their range starts at zero. Empty datasets and
zero-total pies display “No data.” Constant ranges and a single line sample are
supported. `Style.Height` defaults to 240; chart label text defaults to 12 pixels.

`SetChartData` validates and copies a complete snapshot, then repaints that chart
without measuring the surrounding layout. Getters return copies, and invalid
updates leave the previous chart intact. Unchanged snapshots do not repaint.
`ChartOptions` can override `Range`, `Palette`, `LineWidth`, `HideLegend` and
`HideGrid`; a series `Color` overrides the palette except for pie slices, whose
colors come from the category palette. `ChartGridPart`, `ChartAxisPart` and
`ChartLabelPart` customize named parts. Legends reserve bounded space and shorten
to a prefix plus a remaining-item count when necessary.

Each chart accepts up to 128 samples and eight series, with labels/names up to
128 UTF-8 bytes. Pie charts allow up to 32 slices and radar charts 3–16 axes.
NaN/infinite values and mismatched data are rejected. Charts are read-only,
contribute no focus stops and expose a cached textual data summary through
`Semantics`; an application description can be supplied with `SetLabel`.
This does not add a platform screen-reader bridge. The [Charts playground](../../samples/ui/)
shows all four types, data updates, signed bars and zero/single-source pies.

Rows/columns express layout relationships: padding, spacing, fixed/min/max
width, fixed height and remaining-space growth. `Style.Basis` supplies a preferred
row-child width; `Wrap` on a Row starts another line when those children do not
fit. For example, these fields share a wide row and stack on narrow viewports:

```go
name := ui.NewInput("Display name", "Explorer", "Your name", nil)
preset := ui.NewSelect("Quality", []ui.Option{
    {Key: "balanced", Label: "Balanced"},
    {Key: "high", Label: "High quality"},
}, "balanced", nil)

nameField := ui.NewField("Display name", name).
    WithStyle(ui.Style{Basis: 280, Grow: 1, Gap: 6}).
    WithHelpText("Shown to other players.")
fields := ui.Row(
    nameField,
    ui.NewField("Quality", preset).
        WithStyle(ui.Style{Basis: 280, Grow: 1, Gap: 6}),
).WithStyle(ui.Style{Wrap: true, Gap: 16})
root := ui.NewForm(fields, ui.NewButton("Save", nil))
```

`NewField(label, control)` and `NewForm(content...)` package common form
relationships. Fields keep the label,
control, help and validation message together; `SetHelpText` and `SetErrorText`
update those messages. Errors expose invalid state and descriptive relationships
in the semantic snapshot. Validation rules belong to the application.

Use `WithStyle`, `WithLabel`, `WithTooltip`, `WithButtonVariant`,
`WithButtonVisual`, `WithPanelBackground`, `WithChartOptions` and `WithHelpText`
for construction. They retain the first failure, even when later configuration
succeeds. `ui.New` validates the complete detached tree, so setup has one error
boundary; adding a new subtree to a mounted panel reports its construction
errors through `Add` before attaching anything. Mount errors include indexed
child paths. `Node.Err()` makes retained construction failures inspectable
before mounting, including errors from a helper accidentally used on a mounted
node.

For live updates use the corresponding error-returning `Set` methods. They
report failure immediately and leave invalid updates unapplied; those failures
do not become retained construction errors. A successful later setter does not
clear an earlier construction error. Use `errors.Is(err, ui.ErrBounds)` to
classify invalid values and resource limits. Common node setters also return
`*ui.ValidationError`, inspectable with `errors.As`, with the operation, field,
rejected scalar/byte count and reason. Diagnostics do not copy text/image payloads.

A Row retains its horizontal direction when other style fields are changed. Style values
replace the previous style; zero width stretches and zero height is intrinsic.
Zero gap/font size uses the theme default. Wrapped horizontal flow is a small
layout contract, not CSS compatibility, grid layout or arbitrary stacking.

Scroll regions clip their children and reveal keyboard-focused controls. An
overflowing panel contains wheel input at its boundaries, keeping fast bursts
from moving the outer page. Panels without overflow pass wheel input to an
eligible ancestor. Wheel targets ease over 140 ms; retargeting follows new wheel
input. Touch dragging and keyboard scrolling stay synchronous with direct input
and can hand boundary scrolling to an ancestor. `Node.ScrollOffset` reports the
displayed offset.
`View.SetReducedMotion(true)` finishes pending easing and uses immediate wheel
updates. The browser wrapper forwards `prefers-reduced-motion`; other wrappers
or applications can supply the preference explicitly. Scrolling updates placement
without measuring or reshaping unchanged content. Active modal scopes center
above a WebGPU backdrop, contain focus and restore it when closed.

| Element    | API and behavior                                                                                                                                                                                                                                                   |
| ---------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| Tabs       | `NewTabs(label, []Tab, selectedKey, onChange)` retains all content, showing one panel. Arrow keys and Home/End activate enabled tabs; one tab participates in Tab traversal. `SetActiveTab` selects silently.                                                      |
| Separator  | `NewSeparator` draws a horizontal rule and adds no focus stop.                                                                                                                                                                                                     |
| Tooltip    | `SetTooltip` supplies supplementary help. After 150 ms on hover or keyboard focus, help fades in over 120 ms (immediate with reduced motion), wraps, stays inside the viewport and flips above the target when needed. It remains hoverable, never takes focus and dismisses with Escape or a new interaction.           |
| Alert      | `NewAlert(tone, title, message)` uses `InfoAlert`, `SuccessAlert`, `WarningAlert` or `DangerAlert`, with visible severity text as well as color. Alerts persist until the application hides/removes them; they do not steal focus.                                 |
| Progress   | `NewProgress(label, fraction)` and `SetProgress` use a finite fraction from 0 to 1. The indicator is read-only; compose a Text label/percentage when visible wording is wanted. Indeterminate progress is not implemented.                                         |
| Pagination | `NewPagination(label, page, pageCount, onChange)` uses one-based pages, a bounded number window and first/previous/next/last actions. Boundary actions disable automatically. `SetPage` and `SetPageCount` are silent; changing the count clamps the current page. |

`View.SetTheme` supplies common colors/metrics, action palettes and
`Info`/`Success`/`Warning`/`Danger` feedback colors. `SetPartStyle` customizes track,
thumb, indicator, label, popup, option and tooltip paint for explicit state
combinations. `SetText`, `SetValue`, `SetChecked`, `SetSelected`, `SetActiveTab`,
`SetPage` and `SetProgress` are silent programmatic updates. Interactive changes
call the registered typed callback. Commit and cancellation are separate
notifications. Text callbacks include a `Composing` flag.

Stable Node identity preserves focus, editing selection, scroll position and
prepared resources, including content temporarily hidden by a tab. `Remove` ends
the mounted lifetime; stale IDs cannot target a replacement node. `Add` validates
cycles, ownership and resource bounds before attaching. Nested `Dispatch` is
rejected; callbacks can safely update/remove nodes. Preview/default/bubble
routing separates propagation from default suppression.

## Rendering and assets

Everything visible is rendered through the shared WGSL UI pipeline: rounded
panels, borders, prepared images, MSDF text, selection/carets, focus rings,
scrollbars, popup options, tooltips, feedback indicators and modal backdrops.
No HTML controls or CSS painting are used. UI is composed over the world on the App's existing device/submission.

Font atlases are prepared offline; the runtime never rasterizes glyphs or repacks
an atlas. The optional Go Regular face includes its complete font glyph-ID set,
including substitution glyphs. Unknown text remains in the input and displays
the font's missing-glyph shape. The default face does not provide universal
Unicode or color-emoji coverage. `ui.LoadFont` admits the prepared AFNT0001 pair,
checks its exact dimensions/record bounds and verifies font/page SHA-256 bindings.

Font parsing happens once when loading a face. HarfBuzz shaping and line wrapping
operate on changed dynamic text/metrics, with a bounded 1,024-entry LRU cache. Eviction removes one cold layout rather
than clearing hot labels; unwrapped controls share shaping across widths. Fully
clipped glyphs are omitted from draw records. The
frame path retains record slots, GPU buffers and texture pages. Paint changes
rewrite affected blocks; layout changes compare resolved geometry and retain
unchanged blocks. The backend retains a dense submission of active records, merging neighboring
same-page painter spans across node slot boundaries. Changed revisions compare
packed bytes and upload changed contiguous ranges; idle frames skip packing and
uploads. Skipped source revisions are compared against the backend's last
submission, while a new GPU buffer receives a complete upload.
Removed blocks are reusable and unused pages retire on a changed submission.

Bounds are 4,096 nodes, depth 32, 4,096 UTF-8 bytes per string, 1,024 options per
select, 64 tabs per tab group, up to 1,000,000,000 pages with a constant-size
pagination window, 65,536 retained record slots and 128 MiB of prepared
image/font pixels per View. Overflow/invalid data returns an error. Structural/viewport changes
still measure the bounded layout tree; ordinary scrolling traverses placement
while reusing measurements and shaped text. Virtual lists bound row creation,
binding and placement to their clipped viewport/pool rather than logical count. CPU layout, changed text and platform/driver
work can allocate. No whole-program allocation/FPS guarantee is
made.

## Platform contract

Wrappers forward ordered pointer, wheel, key, editing, composition, focus and
viewport data to `View.Dispatch`. Pointer presses survive between rendered
frames. Release/cancel, nested scrolling and drag ownership are handled in Go.
Pointer capture, clipboard and text-service geometry are platform requests.
Mouse, keyboard and explicit semantic actions use the same control handlers.
`View.CursorAt` resolves pointer, text and directional manipulation cursors.
Wrappers update the canvas/window cursor on pointer or UI changes and retain
the manipulation cursor during capture; idle frames reuse the previous result.

The browser wrapper uses **EditContext** attached to the existing canvas. It
creates no hidden input, textarea or accessibility DOM mirror. A browser without
EditContext reports text input/composition as unavailable via `Capabilities`;
the library does not pretend that printable key events provide a complete editor.
Clipboard operations can be denied by the platform. The Linux native wrapper
uses SDL text/composition events and clipboard services. Native hardware
presentation is currently blocked locally by a failure in adapter callback
initialization with the selected WebGPU binding/runtime pair, before UI execution.
Native compilation and browser execution exercise separate targets.

UTF-8 byte offsets are the Go editing contract. The browser wrapper converts
UTF-16 offsets explicitly; SDL composition offsets are converted from characters.
Editing sessions and document revisions reject stale clipboard results. Controls
support selection, undo/redo, grapheme deletion and text-service composition.
Physical IMEs, touch keyboards and platform editing conventions require separate
device qualification; synthetic input alone is not that evidence.

`Semantics` provides inspectable roles, names, descriptions, bounds and state,
including tab/panel, field/help/error and current-page relationships.
Native/browser screen-reader bridges are not implemented. Keyboard support and
this projection are not claims of screen-reader support or WCAG conformance.
Advanced semantic collections, other platform preference bridges, text
fallback-font chains, rich text and variable-height virtualized collections
remain future work.

## Sample and checks

[The UI playground](../../samples/ui/) exercises the controls, wrapped forms,
scrolling and modal focus. Build or run it from the repository root:

```sh
mise run test
mise run build-native
mise run build-web
mise exec -- go run ./cmd/engine run web ./samples/ui
```

Focused tests use `mise exec -- go test ./pkg/ui/...` under the pinned mise
environment. The
opt-in `TestUIHardwarePresentation` additionally requires a real Vulkan driver,
matching WebGPU native library and display, and rejects software adapters. Browser
WASM execution, GPU pixels, native presentation and physical-device interaction
require separate checks.
