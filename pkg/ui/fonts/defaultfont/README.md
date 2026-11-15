# Default MSDF font

An optional, immutable 1024 x 1024 RGBA8 MSDF page for Go Regular at 32 pixels/em,
with a six-texel distance range. All 712 font glyph IDs are prepared, including
708 nonempty glyphs. The runtime shapes dynamic text and samples these prepared
glyphs; it performs no rasterization or atlas packing.

The atlas occupies 4 MiB. Text opacity comes from the median of the RGB
distance channels; atlas alpha is not used by the text shader. The tested BC7
encoding visibly distorted glyph edges, so the bundled profile retains RGBA8.

Regeneration uses the normal asset pipeline and the embedded msdfgen WASM library
hosted in Go. It needs no native font tool, C/C++ compiler, JavaScript engine or
package installation. From the repository root, regenerate through the tracked
`asset.config.go` recipe:

```sh
mise exec -- go run ./cmd/engine assets generate --project ./pkg/ui/fonts/defaultfont
```

`ui/build.DefaultFont()` stages glyph metrics and packs the atlas in Go, calls
msdfgen v1.12 through its published WASM C API, validates the complete qualified
pair, then publishes `prepared/atlas.rgba` and `prepared/glyphs.bin` together
through the normal cached asset transaction. The dedicated `prepared/` directory
and its ownership manifest belong to this pipeline; Go source, documentation and
license files remain outside its output. Both payloads and the ownership
manifest stay tracked, so normal Go package builds load the prepared font without
running its recipe. Local pipeline state lives in ignored `generated/`.

The current generator identity is `msdfgen-wasm/1.0.0/goregular-host1`.
The AFNT0001 metrics use the fixed Go Regular profile above. To review regeneration,
use a separate checkout and run the same command; ordinary publication verifies
the complete pair and leaves identical files unchanged. An unchanged invocation
reuses its verified action cache without initializing the font processor.

`ui/build.DefaultFont(DefaultFontOptions{Format: texture.BC7})` remains an
explicit offline alternative, with `ui.LoadPreparedFont` for admission. It changes
only atlas bytes and their identity hash in AFNT0001, requires device BC support,
and needs application-specific text-quality approval. It is not the qualified
bundled representation. The runtime never decodes or transcodes it.

The font source is `golang.org/x/image/font/gofont/goregular` from the pinned
x/image dependency. The [Go font license](LICENSE) is included locally; retain
its notices with distributed font payloads. Library source pins, licenses,
profile and bounds live in
[the embedded library notices](../../../../internal/msdfcodec/third_party/README.md).
The preparation code and WASM library are never linked into game applications.
