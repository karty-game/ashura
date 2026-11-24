# UI Components

A retained WebGPU interface demonstrating controls, forms, game artwork, charts,
interaction motion, decorated panels and virtual lists. Every section uses the
same UI library on browser and native targets.

```sh
mise exec -- go run ./cmd/engine run web ./samples/ui
```

Choose a section with the tabs. Tab/Shift+Tab navigate; Enter/Space activate;
arrows adjust values and switch tabs. Wheel or touch scrolls. **Test modal focus**
opens a dialog; Escape or Close restores focus to its opener. Diagnostics start
collapsed; use Debug or F3.

The **Layout** section demonstrates inline validation. **Performance** switches
between 10,000 and 100,000 logical rows while retaining a viewport-sized control
pool. Frame charts use illustrative data rather than live GPU measurements.

`main.go` owns the App; `setup.go` loads the prepared font and images;
`interface.go` composes the sections. Feature files keep each example separate.
`assets.go` loads one shared image catalog. Source PNGs in `assets/ui/` flow through
`Images → Atlas → ui/build.Images` in [asset.config.go](asset.config.go).
There is no sample-specific generator, runtime image decoding or atlas packing.

Read the [feature walkthrough](features.md) for button composition, charts,
motion, panel decoration, asset preparation and virtual-list details. The
[UI package guide](../../pkg/ui/README.md) documents the public API and ownership.

Browser text editing requires EditContext; native text editing uses SDL services.
The prepared default font has Go Regular's coverage; unsupported glyphs retain
their text and show a missing-glyph shape. A screen-reader bridge is not
implemented. Physical IMEs, touch keyboards and native-window support require
separate qualification.
