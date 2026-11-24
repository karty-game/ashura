# Authored decal build plugins

Use decalbuild.BakeAlbedo for permanent marks and decalbuild.Projectors for
marks that need an independent state in the game. Both use the same
world-space projector position, outward normal, box size, angle, tint,
receiver mask and blend order.

## Bake permanent marks

Place BakeAlbedo after assetbuild.Images and before mip generation and native
texture encoding. Its Receiver and each Mark.Image name standalone albedo
resources. Supply the receiver triangles in world space with unique normalized
albedo UVs, counter-clockwise vertex order and the intended receiver layer mask.
Atlas packing can follow the bake. The processor composites in linear light
and preserves the receiver's alpha. It regenerates only the base image;
subsequent pipeline stages prepare mip levels and GPU formats.

Receivers need their own UV area wherever the markings differ. Give the
receiver's UV islands sufficient padding for the mip levels used by the level.
Use the generated albedo on the material when displaying that receiver.

## Prepare toggleable projectors

Place Projectors in the recipe output before KTX2 and GoCatalog. It references
the image set produced by assetbuild.Atlas. Its Atlas identifies a single
sRGB albedo atlas page, and each Mark.Image names the atlas region's source
file. Name each batch with an exported Go identifier. A batch may contain up
to 1,024 marks. The build publishes its versioned GPU records and a generated
constructor under the decals directory.

Load the atlas through the ordinary prepared texture catalog and decode each
batch once. Associate the batch with the chunk that owns its placement.
Pass the same atlas handle to live marks and mounted batches. For the complete
recipe and all three types of marks, see the
[Decals sample](../../../samples/decals/README.md).
