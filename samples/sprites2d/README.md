# Abyssal — a 2D field study

An illustrated midnight-ocean scene built entirely with the retained `draw2d`
API. A luminous jellyfish drifts above a research submersible, schools of fish
cross the scene, bubbles and plankton rise through the water, and dense kelp
beds and scattered pebbles give the reef depth. The default scene contains
**1,620 retained sprites**: 640 kelp stems, 320 bubbles, 384 plankton motes,
96 fish, 160 seabed details and 20 scene/sonar elements.
The interface uses the shared retained UI library.

```sh
mise exec -- go run ./cmd/engine run web ./samples/sprites2d
```

Click or tap the water to relocate the sonar. **Motion** or **Space** pauses the scene;
**Depth** reveals the contribution of the foreground, distant reef, kelp, bubbles
and particles.
**Scan** clips sonar rings inside a marked survey window. The bioluminescence
selector changes tint without reloading any images. F3 opens the engine panel.
Controls remain available while motion is paused.

## What the scene demonstrates

| Feature | Visible use |
| --- | --- |
| Prepared standalone image | Full-canvas ocean illustration |
| Shared atlas regions | Every organism, reef, glow and sonar ring |
| Painter order | Background, organisms, sonar, foreground, retained UI |
| Source-image animation | Equal-sized authored jellyfish and fish frames, changed with `SetImage` |
| Pivot and rotation | Jellyfish and submersible drift around local centers |
| Negative scale | Left-facing fish and a mirrored coral silhouette |
| Tint and opacity | Bioluminescence palettes, distant fish and fading sonar |
| Clipping | Framebuffer-aligned bounds truncate expanding sonar rings |
| Visibility | Depth layers and scan markers toggle without destroying sprites |
| Retained updates | Startup sprites are reused; pausing stops animated transform/frame updates |
| Responsive coordinates | Composition follows framebuffer dimensions; logical pointer events convert with `FromLogical` |

This is an illustrated scene with arbitrary transforms, rather than a pixel-grid
fixture. Nearest sprite sampling remains the API's sampling policy. Sprites have
no camera/depth/lighting transform; all composition is authored in screen space.
The sample adds no shaders, renderer extensions or runtime image processing.

## Artwork and build

Original PNG artwork lives in [assets/](assets/). The standard recipe
imports those sources with `Images`, packs the `sprites/**` images with `Atlas`,
and publishes with `draw2d/build.Images`. The ocean background stays standalone.
There is no sample artwork generator or separate preparation command.
The recipe encodes the opaque 1280×800 background as BC1/ETC2RGB and the
2048×1024 sprite atlas as BC7/ETC2RGBA, preserving smooth alpha, source resolution
and atlas layout. The selected native payloads require the corresponding
device compression feature.

The bubble is 32×32, kelp is 48×160 with its root at `(24,160)`, and the pebble
is 40×24. Those source dimensions and pivots are part of the authored composition.
The PNGs preserve the earlier code-authored composition; edit them with an image
editor. Equal-sized jellyfish and fish frames are independent artwork variants.

```sh
mise exec -- go run ./cmd/engine assets generate --project ./samples/sprites2d
mise exec -- go run ./cmd/engine assets build --project ./samples/sprites2d
```

Prepared BC and ETC2 pages and named catalog code remain ignored. The recipe
publishes desktop BC1/BC7 and mobile ETC2RGB/ETC2RGBA catalogs with identical
dimensions and source rectangles. On the first update, after device creation,
the sample queries `App.CompressedAlbedoFormat()` once and loads only that
catalog. Devices without BC or ETC2 are rejected explicitly. PNG decoding,
packing, gutter extrusion and compression are build work; there is no runtime
transcoding. Both variants count toward the complete sample export budget.
Fish, plankton, bubbles, kelp, pebbles, rings and markers are fixed-size pools.
Kelp sways slowly around fixed roots with independent current phases; bubble
sizes and rise rates vary by depth.
Pebbles and other static elements update only on resize, and hidden habitat
layers skip animation. Sprite count and image residency stay fixed during motion.
Image frames change twice per second; all motion reuses retained sprite handles.
The informational readout updates at most twice per second.

`main.go` owns startup/error cleanup; `scene.go` loads the catalog and composes
an empty 3D world; `sprites.go` owns retained objects and animation; `sonar.go`
owns scan geometry, palette and layer controls; `interface.go` builds the UI.
See the [shared sample conventions](../README.md) and
[draw2d contract](../../pkg/draw2d/README.md).

## Copyright annex

The original Abyssal PNG illustrations (background, organisms, submersible,
reef and effects) use the repository's [MIT license](../../LICENSE).
