# Embedded xatlas library

`xatlas.wasm` is the unchanged module from the published
[`xatlas-wasi@0.0.2`](https://www.npmjs.com/package/xatlas-wasi/v/0.0.2) package.
Its wrapper source is pinned by the package's `gitHead` to
[`5eb6ec7a51c400aac4646b297e4c043c643775d0`](https://github.com/kzhsw/xatlas-wasi/tree/5eb6ec7a51c400aac4646b297e4c043c643775d0).
This is a third-party xatlas fork; it is not claimed to be the former native
helper's upstream revision or to produce identical atlas bytes.

- Package archive: <https://registry.npmjs.org/xatlas-wasi/-/xatlas-wasi-0.0.2.tgz>
- Archive SHA256: `cb82aa35ebdf2b0ff1f84a1c5967a46ac3dad2561c58ab3d0c8880618aa8ae51`.
- npm integrity: `sha512-9/zQDdo2XKgLWPNMQBLdNVZfcTJCTA2CFNixESNamCSUOEE/vBCRjiPzsoytTFM+0noBHB4kYCYReJ910oc6yA==`.
- `xatlas.wasm`: 106,113 bytes, SHA256 `0036793f9233f4e2ab190cf300c644836147d79fb972d1a13c0214c0e1af5761`.

The archive's integrity was verified before extraction. The Go host also verifies
the embedded module hash before compiling it. The only host import is the
progress/cancellation callback `env.js_progress`; no WASI filesystem or network
capability is supplied. No JavaScript glue, native helper or consumer C++ build
is required. Compiled code is reused; each request owns an isolated instance.

`LICENSE` retains xatlas's MIT notice and `LICENSE-wrapper` retains the wrapper's
MIT notice. The module is unmodified. Overlap/collapse repair and bounded host
admission are implemented in Go beside `wasi.go`. See [lightmap UV preparation](../../../docs/asset-pipeline/processors/level-lightmap-uvs.md).
