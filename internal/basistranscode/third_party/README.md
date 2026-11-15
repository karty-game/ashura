# Basis Universal transcoder

Unmodified prebuilt web transcoder from BinomialLLC/basis_universal v2_50,
commit `9bebe16726b3a61c8c213eeee3b7cffb462ef34e`.
Source archive:
`https://codeload.github.com/BinomialLLC/basis_universal/tar.gz/9bebe16726b3a61c8c213eeee3b7cffb462ef34e`
SHA256 `8f4397a8d547ab07736c34a8d835e783cd019c3a0bd83d343c1bc4ebe7c570df`.

Files come from `webgl/transcoder/build/`:

- `basis_transcoder.js`: SHA256 `720dd9bd09c7cada6d87f1b7b70cec713df04da88cd641ac3212559353834dc8`.
- `basis_transcoder.wasm`: SHA256 `a0f65d4a30ecb3269d01ead7d0a3477d2b0208146d083625a90623f473f6c139`.

Retain the upstream LICENSE and NOTICE. Ordinary builds embed these immutable
files and do not download tools. The parent worker is handwritten integration
code; it transcodes opaque sRGB ETC1S to BC1 or ETC1-compatible ETC2 RGB blocks.
It does not request an RGBA fallback or perform image resizing or mip generation.
