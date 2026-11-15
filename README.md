# Ashura

![Ashura — Go-first WebGPU rendering](branding/banner-readme.png)

A Go-first WebGPU engine built around one mantra:

> Do as much work as possible ahead of time so you have to do as little as possible at runtime.

## Features

- **Browser and native Linux rendering** with a shared Go API and WebGPU backend.
- **3D scenes** with meshes, instancing, materials, lights, shadows and prepared reflection maps.
- **Retained 2D sprites** with prepared atlases, transforms, clipping, tint and opacity.
- **Retained UI** with layout, controls, text editing, motion and virtual lists. Keyboard operation and semantic metadata are supported; platform screen-reader bridges are not implemented.
- **Offline asset preparation** for models, textures, atlases, lighting and reflection maps.
- **GPU compute** with WGSL and explicit buffers.

## Try it

[mise 2026.11.5 or newer](https://mise.jdx.dev/installing-mise.html) is required
for development on Windows, macOS and Linux. It installs the pinned Go and
development tools. UV generation, texture codecs and font preparation use embedded WASM libraries
through Go. Remaining native dependencies use checksum-pinned prebuilt packages
through mise; users do not need GCC, Clang, Cargo or a separate tool installer.
TinyGo has official binaries for Linux amd64/arm64, macOS arm64 and Windows
amd64. Mise also installs checksum-pinned upstream Binaryen 133 on Windows,
as required by [TinyGo's Windows WebAssembly compiler](https://tinygo.org/getting-started/install/windows/).

```sh
mise install
mise run start
```

Open the printed URL in a WebGPU-capable browser. This starts the basic scene
without native asset tools. The full gallery (`mise run samples`) also prepares
Sponza using embedded UV/codec libraries and native hardware compute. See
[dependency availability and qualification limits](spec/asset-tools.md).

On Linux, Sponza's offline bake requires the system Vulkan loader and your GPU's
Vulkan driver. For AMD/Intel on Debian or Ubuntu, install `libvulkan1` and
`mesa-vulkan-drivers`. Mise installs wgpu-native; the system packages supply the
Vulkan runtime and hardware driver it uses. Browser rendering and a working
OpenGL driver do not establish that this native build dependency is installed.

[API guide](pkg/engine/README.md) · [Samples](samples/README.md) · [First scene](samples/basic-scene/) · [UI Components](samples/ui/) · [Asset recipes](pkg/assetbuild/README.md) · [Specifications](spec/README.md)

## Repository layout

- `pkg/`: public Go packages.
- `internal/`: private rendering, platform and preparation implementation; see the [subsystem map](internal/README.md).
- `cmd/engine/`: the build, asset and preview CLI; `samples/`: runnable examples and source assets.
- `spec/`: maintained contracts, proposals and validation; `branding/`: artwork.

Use `mise run check` for the complete source gate and `mise exec -- <command>`
for focused commands. Read [contributor instructions](AGENTS.md) before changing
library code. [Offline libraries and tools](spec/asset-tools.md) describes the
portable UV/codec/font modules, upstream native GPU library and remaining native
dependencies. Lightmap baking runs in Go using the same binding as
native rendering; it has no separate worker executable or release.

Source code is available under the [MIT license](LICENSE). Bundled artwork,
models, fonts and other third-party components retain their own notices;
sample-specific attribution lives in each sample README's copyright annex.
