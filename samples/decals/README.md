# Decals

Spray paint onto the Switchyard walls and freight car. Toggle **Yard decals**
to see which details are projected onto the surfaces. The large pavement arrows
and floor stencil are baked into the asphalt texture.

Run this command from the repository root to open the browser preview.

```sh
mise exec -- go run ./cmd/engine run web ./samples/decals
```

## Controls

| Action | Keyboard and mouse | Touch |
| --- | --- | --- |
| Spray | Left-drag on the mural wall, service bay or freight car | Drag one finger |
| Move the view | Right-drag to orbit, scroll to zoom | Two-finger drag to pan, pinch to zoom |
| Choose a paint color | **1–3** or the palette buttons | Tap a palette button |
| Clear your paint | **R** or **Clear** | Tap **Clear** |
| Toggle the debug panel | **Debug**, **Hide**, or **F3** | Tap **Debug** or **Hide** |

Your paint stays until you clear it or reload. The **Yard decals** switch
only toggles the authored details.

## Code and assets

`asset.config.go` prepares the atlas, the collision mesh, 649 authored decals
and the spray grid. `spray.go` casts a ray into the yard and toggles the prepared
decals along the stroke. `interface.go` builds the palette and displays the
paint count. See the [projected decal API](../../pkg/decal/README.md).

On Linux, these commands prepare the assets and start the native sample.
The native sample uses Ashura's display and WebGPU dependencies.

```sh
CGO_ENABLED=0 GOFLAGS=-tags=nofakecgo mise exec -- go run ./cmd/engine assets generate --project ./samples/decals
CGO_ENABLED=0 GOFLAGS=-tags=nofakecgo mise exec -- go run ./samples/decals
```

## Copyright annex

The original fox mural was generated with ImageGen. [Mural PNG](assets/fox-mural-albedo.png)

<details>
<summary>Mural prompt</summary>

> Create a genuinely transparent PNG game decal cutout. Original underground railway-yard graffiti mural featuring a stylized electric teal fox head with alert orange eyes, angular dark navy outlines, pale cyan highlights, coral and marigold arrow shapes, layered aerosol overspray, little stars and deliberate drips. Bold hand-painted street art, authentic spray grain, legible high-contrast shapes from a distance. Wide horizontal standalone mural, centered with generous transparent margins. It will be projected onto a concrete freight-yard wall in an interactive 3D demo. Only pigment and overspray on transparent alpha. No wall, no backdrop, no photograph, no borders, no perspective, no text or logos.

</details>
