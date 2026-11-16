# Prepare the default font

[Asset pipeline](../README.md) · [Generators](README.md)

`uibuild.DefaultFont()` rebuilds the bundled Go Regular font atlas and glyph
metrics. Ordinary UI applications can use the existing bundled font directly;
use this generator when you need to regenerate its prepared files.

Go packs the glyphs and [msdfgen](https://github.com/Chlumsky/msdfgen), running as
embedded WASM, computes their distance fields. Those fields let the renderer
draw smooth glyph edges at different sizes. This plugin supports the fixed Go
Regular profile; it is not an importer for arbitrary fonts.

Import `uibuild "github.com/karty-game/karty-webgpu/pkg/ui/build"`.

## Settings and defaults

Pass no arguments or one `uibuild.DefaultFontOptions` record.

| Setting | Default | Meaning |
| --- | --- | --- |
| `Format` | `texture.RGBA8` | Exact 1024×1024 RGBA8 atlas, 4 MiB. `texture.BC7` selects a 1 MiB compressed atlas with unchanged glyph layout. Other formats are rejected. |

The font, glyph roster, 32 pixels/em, six-texel range, InkTrap angle 3 and metrics
format are fixed rather than recipe settings. BC7 requires device BC support;
the bundled font uses RGBA8 because BC7 showed visible glyph edge distortion.

## How to use it

This plugin needs no source `Input` or other processing stages. A standalone
recipe is:

```go
//go:build ignore

package main

import (
    "github.com/karty-game/karty-webgpu/pkg/assetbuild"
    uibuild "github.com/karty-game/karty-webgpu/pkg/ui/build"
)

func main() {
    assetbuild.Main(func(assetbuild.Environment) assetbuild.Config {
        return assetbuild.Config{
            Output: "prepared",
            Pipelines: []assetbuild.Pipeline{{
                Name: "font", Output: []assetbuild.Plugin{uibuild.DefaultFont()},
            }},
        }
    })
}
```

The plugin publishes `atlas.rgba` and `glyphs.bin` as a matched pair; BC7 writes
compressed blocks to the same atlas filename. It does not generate a Go catalog.
For custom consumption, use `ui.LoadPreparedFont` with the corresponding
`texture.Format`. Keep the files together when embedding or distributing them.

The repository's bundled font already embeds its prepared pair, so ordinary UI
applications do not need to rebake it. To regenerate that owned pair, from the
repository root run:

```sh
mise exec -- go run ./cmd/engine assets generate --project ./pkg/ui/fonts/defaultfont
```

See the [bundled font guide](../../../pkg/ui/fonts/defaultfont/README.md) for
runtime font construction and its fixed profile.
