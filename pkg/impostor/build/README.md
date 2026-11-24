
For mobile view captures, set `AtlasFormat: texture.ASTC6x6` (six-aligned base
atlases) or `texture.ETC2RGBA` (four-aligned). Both retain RGB normals and depth
alpha. Layered numeric captures remain RGBA8. Capture resolution and authored
mips are preserved; choose padding that satisfies the mip and block alignment.

`TextureProfile: "bc"` or `"mobile"` publishes GPU-ready `.kimp` files under
`generated/public/texture-profiles/<profile>/` and descriptors under
`generated/impostors/<profile>/`. Generated functions return `resource.Asset`
with path, logical size and SHA-256. Read only the supported profile through
`resource.Read` on an I/O goroutine, then call `impostor.Prepared(...).Load(app)`
on the application loop. Empty `TextureProfile` retains embedded catalogs.
