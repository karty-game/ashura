# Prepared UI assets

`ui/build` prepares authored PNG and JPEG artwork offline. `uibuild.Atlas`
imports the images, packs the selected group, and emits GPU-ready pages with
a typed UI catalog.

```go
//go:build ignore

package main

import (
    "github.com/karty-game/ashura/pkg/assetbuild"
    uibuild "github.com/karty-game/ashura/pkg/ui/build"
)

func main() {
    assetbuild.Run(assetbuild.Config{
        Output: "generated/assets",
        Pipelines: []assetbuild.Pipeline{
            uibuild.Atlas(uibuild.AtlasOptions{Name: "ui", Include: "ui/**", MaxSize: 512, Padding: 2}),
        },
    })
}
```

Put the recipe in `asset.config.go`. `engine assets generate` runs it directly,
and `engine run web` runs it before compiling the game. For custom input roots
or resizing, compose the explicit image stages and use `uibuild.Images()` as
the publisher. Nine-slice border cuts use the resulting image dimensions. The
UI publisher consumes one albedo RGBA8 level per image or page.

For `assets/ui/gold-frame.png`, the generated runtime API is:

```go
images, err := assets.Load()
if err != nil {
    return err
}
if err := panel.SetPanelBackground(ui.PanelBackground{
    Image: images.UiGoldFrame,
    Slice: &ui.NineSlice{Left: 32, Top: 32, Right: 32, Bottom: 32},
}); err != nil {
    return err
}
```

Generated `Catalog` fields use the pipeline's original image names. A load admits
each page once with `ui.NewImageAsset`, then derives its named regions without
copying their pixels. Call `assets.Load` once for the asset lifetime and share the
catalog; repeated loads intentionally create independent owned pages. Standalone
images and multiple atlas pages use the same interface. Source dimensions and
atlas positions come from generated metadata; runtime callers do not hardcode
UVs or packed coordinates.

Output contains `assets.go`, tightly packed straight-alpha `pages/*.rgba`, and
`atlas.json` for packed images. PNG decoding, atlas packing, gutter extrusion and
all image transforms finish during the build. The plugin remains host-only;
generated image loading never decodes source images or repacks an atlas. Keep
recipes and independent source images;
ignore generated output and build caches as described by the asset pipeline.

## Bundled Go Regular font

`uibuild.DefaultFont()` publishes the qualified `atlas.rgba` and `glyphs.bin`
pair in a separate pipeline. Go stages glyph metrics and packs the page; an
embedded msdfgen v1.12 WASM library generates the distance fields through
Go/wazero. No native font tool, package installation, compiler, JavaScript engine
or executable path is required.

Preparation checks source/page identities and exact retained output hashes, then
publishes the pair through the normal cached transaction. The fixed source and
profile preserve Go Regular at 32 pixels/em, InkTrap angle 3, six-texel range,
glyph roster, rectangles and AFNT0001 layout. This plugin does not claim
arbitrary-font import. The regenerated pair corrects the previous U+222A UNION
sign inversion; seed changes concern MSDF distance-channel assignment, not text
color. Independent Go outline coverage provides regression evidence.

One operation owns a bounded instance; cancellation returns no partial pair.
Warm-cache reuse initializes no module. The runtime uses prepared glyphs and
performs no rasterization or packing. See the embedded font module's
[provenance and limits](../../../internal/msdfcodec/third_party/README.md).

The bundled font owns a normal `asset.config.go` recipe with `Output: "prepared"`
and one pipeline whose output is `uibuild.DefaultFont()`. The bundled font keeps
RGBA8 after actual GPU-decoded BC7 showed visible glyph edge distortion. The
optional `DefaultFontOptions{Format: texture.BC7}` compresses offline to 1 MiB
without changing dimensions or glyph metrics, but is not the qualified bundled
representation. RGB median distance controls text opacity; font atlas alpha is
unused. Compressed output requires device BC support and application-specific
text-quality review; there is no runtime conversion. Regenerate from the
repository root with:

```sh
mise exec -- go run ./cmd/engine assets generate --project ./pkg/ui/fonts/defaultfont
```

The bundled publisher owns only `prepared/atlas.rgba` and `prepared/glyphs.bin`, recorded
in the tracked `prepared/.engine-assets.json` manifest. Its cache stays in ignored
`generated/`; source, documentation and licenses sit outside the publication
directory. The tracked pair is embedded by the runtime package, so normal Go
package builds require no generation. See the [font guide](../fonts/defaultfont/README.md).
