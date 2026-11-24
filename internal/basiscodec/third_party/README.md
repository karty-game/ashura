# Embedded Basis Universal libraries

These are unchanged upstream Basis Universal **2.50** single-threaded C-API
WebAssembly modules from commit
[`9bebe16726b3a61c8c213eeee3b7cffb462ef34e`](https://github.com/BinomialLLC/basis_universal/tree/9bebe16726b3a61c8c213eeee3b7cffb462ef34e).
They execute through wazero in the offline Go asset pipeline. No JavaScript
wrapper or native executable is bundled or required.

| Local file | Upstream path under that commit | Bytes | SHA256 |
| --- | --- | ---: | --- |
| `encoder.wasm` | `python/basisu_py/wasm/basisu_module_st.wasm` | 7,165,939 | `6bb6dd89c28e0bc1b639da2b897781de753d5b50e6ddbbf287212182c3a2b0b8` |
| `transcoder.wasm` | `python/basisu_py/wasm/basisu_transcoder_module_st.wasm` | 2,347,682 | `55d9480cca82d8d97c5a18eaf8e4debfbdc5667da6729eab5a85efc059e5a4e9` |

The corresponding source archive is
[`9bebe16726b3a61c8c213eeee3b7cffb462ef34e.tar.gz`](https://github.com/BinomialLLC/basis_universal/archive/9bebe16726b3a61c8c213eeee3b7cffb462ef34e.tar.gz),
SHA256 `8f4397a8d547ab07736c34a8d835e783cd019c3a0bd83d343c1bc4ebe7c570df`.
Fetch updates from a specific upstream commit, verify exact bytes and retain
notices. Do not build or download these libraries during consumer preparation.
The Go host verifies each embedded hash before initializing its module.

`LICENSE`, `NOTICE`, `LICENSES/` and `copyright` retain upstream licensing and
attribution, including its component exceptions. The upstream source is
unmodified. Host-side admission, channel handling and orchestration are Go code
in this repository. See [codec profiles and portable library requirements](../../../pkg/assetbuild/block_compress_tools.md).
