# Prepared 2D images

`draw2d/build.Images()` turns independently authored images into a typed
`assets.Catalog` containing `*draw2d.Image` handles. Source decoding, atlas packing
and any image transformations run offline through `assetbuild`.

```go
//go:build ignore

package main

import (
    "github.com/karty-game/karty-webgpu/pkg/assetbuild"
    spritebuild "github.com/karty-game/karty-webgpu/pkg/draw2d/build"
)

func main() {
    assetbuild.Main(func(assetbuild.Environment) assetbuild.Config {
        return assetbuild.Config{
            Output: "generated/assets",
            Pipelines: []assetbuild.Pipeline{{
                Name: "sprites", Input: []string{"assets/**"},
                Import: []assetbuild.Plugin{assetbuild.Images()},
                Output: []assetbuild.Plugin{spritebuild.Images()},
            }},
        }
    })
}
```

Save this as `asset.config.go`. Run `engine assets build --project .`, or use
`engine run web`, which prepares assets before compiling the game. For
`assets/icon.png`, the generated code provides:

```go
images, err := assets.Load()
if err != nil {
    return err
}
app.Screen2D().NewSprite(images.Icon).SetPosition(10, 10)
```

A 32×32 source stays 32×32. Avoid `Resize` when exact source dimensions matter.
The default sprite transform uses its top-left corner and natural dimensions.

For shared atlas pages, add the existing offline atlas stage to `Process`,
before the sprite output:

```go
Process: []assetbuild.Plugin{assetbuild.Atlas(assetbuild.AtlasOptions{
    Name: "sprites", Include: []string{"sprites/**"},
    Clamp: true, MaxSize: 1024, Padding: 2,
})},
```

This packs the independent sources under `assets/sprites/` while leaving other
images standalone. Named catalog fields retain their original source identities;
no caller needs packed coordinates. Multiple atlas pages work automatically.

Output contains `assets.go`, tightly packed straight-alpha `pages/*.rgba`, and
`atlas.json` when regions are packed. Each `assets.Load()` admits each page once,
then creates lightweight shared regions. Load once per asset lifetime and share
the catalog. Repeated loads intentionally create independent owned pages.

The plugin accepts one albedo RGBA8 level per image. Omit `Mipmaps`, `KTX2` and
`GoCatalog`: this consumer publishes its own ready-to-upload data and catalog.
Empty inputs produce an empty catalog without runtime imports. Keep source
images and the recipe; ignore generated output and the build cache. The generated
runtime catalog imports `draw2d`, without UI controls, image codecs or build tools.

For devices with BC texture-compression support, select native blocks explicitly:

```go
spritebuild.Images(spritebuild.ImagesOptions{
    OpaqueFormat: texture.BC1,
    AlphaFormat: texture.BC7,
})
```

Import `github.com/karty-game/karty-webgpu/pkg/texture` in the recipe. The output
stage checks every page's alpha offline: fully opaque pages use `OpaqueFormat`,
and pages with any nonopaque alpha use `AlphaFormat`. Opaque pages accept RGBA8,
BC1, BC7, ETC2RGB or ETC2RGBA; alpha pages accept RGBA8, BC7 or ETC2RGBA to retain
smooth transparency. Zero
values keep the existing RGBA8 output. `Compression` sets the embedded codec's
quality and per-page timeout.

Compressed page dimensions must be multiples of four; unsupported dimensions
fail the build rather than resizing artwork. Source dimensions, atlas regions,
padding and image coordinates remain unchanged. The output publishes native
`pages/*.bc1`, `*.bc7`, `*.etc2rgb` or `*.etc2rgba` blocks and generates `NewPreparedImage` admission
calls. Loading copies those GPU-ready blocks without decoding, repacking or mip
generation. The device must enable the selected compression family; unsupported
devices reject those pages. For a mixed desktop/mobile application, publish BC
and ETC2 catalogs in separate pipeline directories and load only the supported
one after device creation. Sprites2D uses this pattern. The default RGBA8 output
works without either optional compression family.
