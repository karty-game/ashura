# meshoptimizer 1.0

`encoder.wasm` contains unchanged upstream WASM extracted from
[`js/meshopt_encoder.js`](https://github.com/zeux/meshoptimizer/blob/73583c335e541c139821d0de2bf5f12960a04941/js/meshopt_encoder.js)
at commit `73583c335e541c139821d0de2bf5f12960a04941` (tag `v1.0`).
The upstream distribution was built with clang 19.1.5 / wasi-sdk. Its MIT license
and copyright are retained in `LICENSE`.

- Distribution SHA-256: `e79ba4a73a758f315ab153dc50c389c7b0bea2b5c1fdcfcaa62ad6d0c27cb25c`.
- WASM SHA-256: `57881d86081c08140e6547561d5f43217e94d875f02b22cd89aa36f9d6a3b70a`.
- WASM length: 14,916 bytes.

`extract.go` is the tracked Go maintenance tool that reverses upstream's byte
packing. It neither compiles native source nor executes JavaScript. Fetch the
pinned source, verify its distribution hash, then run:

```
mise exec -- go run internal/meshopt/third_party/extract.go meshopt_encoder.js encoder.wasm
```

The Go host verifies the embedded WASM hash before execution. Consumer asset
builds do not download, compile or install this library. Only vertex-cache and
vertex-fetch optimization exports are used. The module is excluded from game
runtime builds; no encoder, decoder or compression is added to loading.
