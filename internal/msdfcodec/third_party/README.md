# Embedded msdfgen library

`msdfgen.wasm` is the unchanged published `msdfgen-wasm@1.0.0` library,
not a locally rebuilt native adapter. [Provenance](provenance.json) fixes the
archive SHA512 integrity, archive/module SHA256 and package source revision.
The release revision's build script declares **msdfgen v1.12**, matching the
former native adapter, FreeType 2.13.3, libpng 1.6.39, zlib 1.2.13 and Skia
chrome/m133. Retained licenses cover the wrapper and those library components.
These source declarations do not establish a reproducible rebuild of the npm
artifact; the exact distributed module bytes are the executed dependency.

The Go host supplies the module's Emscripten callback/memory ABI directly using
wazero. Its legacy split-i64 `fd_seek` import is not WASI preview1's ABI, so a
bounded host shim denies descriptors instead of linking that function to the
standard WASI implementation. No filesystem, environment, clock, network,
JavaScript engine or external executable is supplied. C++ throws, abort and
longjmp terminate the request. One operation uses one instance, capped at
256 MiB linear memory; authored font bytes, glyph requests and pixels have
additional 16/64 MiB admission limits.

Go stages the fixed Go Regular glyph rectangles and packs both output files.
The C API loads each glyph without Skia preprocessing, normalizes its shape and
winding, uses InkTrap with angle 3 and its default seed zero, and generates a
six-texel range at 32 pixels/em. Go rounds float pixels by `floor(x*255+0.5)`,
clamps unbounded distances, flips rows and writes opaque RGBA8. The package's
JavaScript atlas packing and PNG encoder are unused.

The former adapter used each glyph ID as the coloring seed. RGB distance
channels therefore change, while median distances remain identical or within
one byte for all glyphs except U+222A UNION. The former atlas had that glyph's
sign inverted; the module's winding normalization fixes it. Independent Go
outline coverage is retained as a regression, together with real WASM
cancellation, bounds and complete pipeline/cache checks. This is CPU/WASM
functional evidence, not GPU graphics or device performance evidence.

Portions of this software are copyright © 2024 The FreeType Project
(https://freetype.org). All rights reserved.
