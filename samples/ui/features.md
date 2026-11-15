# UI Components: feature walkthrough

[Run the sample and find its source](README.md).

## Game button composition

The Game UI examples use the same Button and ToggleButton controllers as the
other sections. A visual is decorative: the complete button remains one hit
region and one keyboard focus stop. Symbol-only and image-only buttons provide
an accessible name with `WithLabel` and supplementary help with `WithTooltip`.

```go
button := ui.NewButton("Buy · 250", buy)
button.WithButtonVisual(ui.ButtonVisual{Image: coin, Size: 30})

next := ui.NewButton("Continue", continueAction)
next.WithButtonVisual(ui.ButtonVisual{
    Icon: "›", Side: ui.TrailingContent, Size: 24,
})

slot := ui.NewToggle("", true, selectPotion)
slot.WithLabel("Health potion", "Restores 40 health.")
slot.WithButtonVisual(ui.ButtonVisual{Image: potion, Size: 42})
```

The coin, crystal, potion and shield are original sample artwork, authored as
independent PNGs in [assets/ui](assets/ui/). They share the build-generated image
catalog with the panel artwork; the application loads each prepared page once.
No application code specifies atlas coordinates or embeds a manually packed page.
See [PNG assets and automatic packing](#png-assets-and-automatic-packing) below.

## Charts

The Charts examples show two-series frame timing, grouped positive/negative
round scores, reward shares, and six character attributes against a reference
build. Cards share wide rows and stack on narrow screens. Each chart has a
semantic name, axis/category labels and a text legend; chart surfaces themselves
do not add keyboard focus stops.

**Sample frames**, **Next match** and **Switch build** replace bounded datasets
without rebuilding the interface. **Zero rewards** demonstrates a zero-total pie
with no slices, and **Restore rewards** returns to the original breakdown.
Samples update only when requested; this demo does not continually resample or
animate unchanged data.

```go
chart := ui.NewChart(ui.LineChart, "Frame time in milliseconds", ui.ChartData{
    Labels: []string{"1", "2", "3", "4"},
    Series: []ui.ChartSeries{
        {Name: "CPU", Values: []float64{10, 12, 11, 17}},
        {Name: "GPU", Values: []float64{7, 8, 9, 12}},
    },
}).WithStyle(ui.Style{Height: 250, FontSize: 11})
chart.WithChartOptions(ui.ChartOptions{
    Range: &ui.ChartRange{Min: 0, Max: 33},
})
```

The four chart kinds use the shared WebGPU painter and MSDF labels. Their data
and options are admitted through typed setters; there is no HTML chart widget,
runtime image generation or additional renderer/device.

## Interaction motion

The Motion examples pair primary, secondary and quiet actions at a fixed height,
with a confirmation counter, a selectable Favorite action, an icon-only help
button and an unavailable action. **Confirm action** increments its counter as
soon as it activates. Press and hold to follow the ripple, drag away to cancel,
or use Tab with Enter/Space to try the same controls from the keyboard.

**Player tag** and **Session note** show focus moving between inputs. **Validate
tag** checks the tag length; Enter inside that field performs the same check.
**Clear validation** restores its help text. Labels remain outside the inputs so
they are always visible while typing and do not shift during focus transitions.

The **Interaction effects** checkbox changes the application's motion setting:

```go
theme := view.Theme()
theme.Motion.Disabled = !enabled
check(view.SetTheme(theme))
```

System reduced-motion preferences still apply independently. Disabling motion
keeps the immediate hover, pressed, focus and validation feedback, without
expanding ripples or intermediate transitions. Motion uses the existing UI frame
clock and preserves the controls' layout, hit regions and text-editing behavior.
Here and in other live callbacks, `check` is the setup-owned error collector's
`Check` method. Construction uses fluent `With*` helpers and checks `ui.New` once;
mounted updates keep their immediate setter errors.

## Panel artwork and 9-slice frames

The Panels examples use ordinary layout panels with a decorative background.
**Switch frame** changes between gold and steel artwork around an illustrated
night landscape. The landscape background and 9-slice border are separate
panels, so the image fills its surface while corner ornaments stay sharp.
**Resize panel** changes the journal's width and height while its corner
ornaments retain their size.
The expedition input keeps its value through both operations. **Show background**
and **Show frame** independently control the landscape and decorative border.
Both start enabled; either can be hidden separately, or both can be hidden.

```go
illustration := ui.Column(title, content).
    WithStyle(ui.Style{Grow: 1, Padding: 12, Gap: 12})
illustration.WithPanelBackground(ui.PanelBackground{
    Image: landscape, Fit: ui.CoverImage,
})
panel := ui.Column(illustration).
    WithStyle(ui.Style{Height: 230, Padding: 16})
panel.WithPanelBackground(ui.PanelBackground{
    Image: frame,
    Slice: &ui.NineSlice{
        Left: 32, Top: 32, Right: 32, Bottom: 32,
        Scale: 0.5, BorderOnly: true,
    },
})
```

Source cuts are image pixels; Scale converts them to logical UI pixels. The
sample's 32-pixel cuts and half-scale produce 16-pixel corners. Its tinted quest
frame uses a larger border with `BorderOnly: true`; a tiny companion frame
shows proportional compression when the panel is smaller than its border pair.
Padding is set separately so controls stay clear of decorative edges.

The same prepared landscape demonstrates `StretchImage`, `ContainImage` and
`CoverImage` in rounded panels. Stretch fills the panel, contain preserves the
complete image with letterboxing, and cover preserves proportions with a
centered crop. The dispatch list demonstrates that a background stays attached
to a scrolling panel while its child controls scroll normally.

Gold/steel frames and the night mountain scene are independent PNG sources in
[assets/ui](assets/ui/). They use the same automatic atlas and generated catalog
as the game buttons. There is no runtime image decoder, pixel resizing or atlas
packing. The panel renderer emits image quads through the existing WebGPU UI
pipeline, with layout, input and accessible control names preserved.

## PNG assets and automatic packing

The seven PNGs in [assets/ui](assets/ui/) are ordinary source assets. Replace or
add PNG files there; [asset.config.go](asset.config.go) imports and packs them
using the standard asset-pipeline plugins before the sample is compiled:

```go
Output: "generated/assets",
Pipelines: []assetbuild.Pipeline{{
    Name: "ui", Input: []string{"assets/**"},
    Import: []assetbuild.Plugin{assetbuild.Images()},
    Process: []assetbuild.Plugin{assetbuild.Atlas(assetbuild.AtlasOptions{
        Name: "ui", Include: []string{"ui/**"}, Clamp: true,
        MaxSize: 512, Padding: 2,
    })},
    Output: []assetbuild.Plugin{uibuild.Images()},
}},
```

`uibuild` is `github.com/karty-game/karty-webgpu/pkg/ui/build`. The UI plugin writes
GPU-ready straight-alpha RGBA pages, a typed `assets.Load()` catalog and
`atlas.json` metadata into ignored `generated/assets/`. The generated catalog
loads each page once and supplies named image regions such as `UiCoin`,
`UiGoldFrame` and `UiLandscape`; changing packing does not require sample-code UV
changes. [assets.go](assets.go) shares one loaded catalog between the button and panel
examples. PNG decoding, packing and gutter extrusion all happen during the build.

`engine run web` invokes the recipe automatically. To prepare assets explicitly
before development, use `generate`; use `build` to refresh playable assets:

```sh
mise exec -- go run ./cmd/engine assets build --project ./samples/ui
mise exec -- go run ./cmd/engine assets generate --project ./samples/ui
```

## Large-list performance

The **Performance** section starts with 10,000 expedition results and can switch
between 10,000 and 100,000 logical rows. It uses `NewVirtualList` with fixed
44-pixel rows and a two-row overscan. The viewport keeps a small pool of ordinary
buttons; scrolling rebinds their current logical row rather than constructing a
control for every result. Clicking a recycled button uses its current bound
index, including after a direct jump to the middle or end of the dataset.

**Update visible row** changes one result. **Update offscreen row** changes a
result at the other end of the collection; its row is bound when it next enters
the mounted range. **New dataset** changes a generation number and refreshes the
mounted rows. Scores are computed directly from the row index and generation,
so neither a dataset replacement nor a jump builds or walks 10,000 row objects.
Application data can also come from an existing model; model loading/sorting is
separate from UI virtualization.

```go
list := ui.NewVirtualList(ui.VirtualListOptions{
    Count: 10000,
    RowHeight: 44,
    Overscan: 2,
    Create: createRow,
    Bind: bindRow,
}).WithStyle(ui.Style{Height: 320, Padding: 8})

// Mutate application data, then invalidate the affected logical range.
check(list.RefreshVirtualRows(index, 1))
check(list.ScrollToRow(9999))
```

**Inspect retained work** snapshots logical/mounted counts, controls created,
row bindings, and whole-playground node/record-slot counts. Inspection is
explicit: the sample does not update labels or allocate telemetry strings on
idle frames. Retained record slots include hidden playground sections and are
not a GPU draw-call counter. These counters demonstrate bounded UI work;
measured CPU allocations, submission counts and hardware frame pacing are
reported separately in [validation](../../spec/validation.md).
