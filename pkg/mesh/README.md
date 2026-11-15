# Mesh conventions

Primitive dimensions are world units in a right-handed frame: +Y up and camera
forward -Z. Geometry is centered at the origin; front triangles wind
counter-clockwise. Box faces have independent vertices/flat normals and [0,1] UVs.
Sphere and capsule use smooth outward normals and duplicate vertices at the UV
seam; U wraps azimuth, V runs pole to pole. Cylinders point along Y, with radial
smooth sides and independent flat caps. Plane is an XZ quad with local +Y normal;
Width spans X and Height spans Z. Billboard is an XY quad with local +Z front;
Width spans X and Height spans Y. Quad/box V zero is the top edge in the authored
face convention. Cylinder cap UVs map X/Z around (0.5,0.5).

All primitive constructors return Mesh directly. Zero dimensions select documented
defaults; nonzero dimensions (including negative/non-finite values) remain caller
responsibility. Segments zero selects 24, and nonzero counts clamp to 3..256.
`NewCustom` alone returns errors: 1..1,048,576 finite positions, a nonempty triangle
index list of at most 3,145,728 indices, bounded indices and matching optional
finite normals/UVs. Construction copies custom slices. Authored geometry is
immutable after attachment; replace slices or the component to change it.
Missing normals use local +Y and missing UVs zero; no runtime normal generation
or geometry rebuilding occurs. Standard derives its tangent frame from UV
screen derivatives; a separate authored tangent stream is outside this mesh API.

Billboard modes are explicit: `BillboardSpherical` faces the active camera with
its up direction, `BillboardAxis` rotates around a configurable world-space axis
(default +Y), and `BillboardFixed` retains the authored transform. Billboard sizes
stay in world units and geometry is shared unchanged while transforms update.
Spherical is the zero/default mode. Instances forward the source mode/axis.

`CreateInstance` retains the source Mesh address and shares immutable geometry.
Entities retain independent transforms/material values. Compatible source/material
instances batch; removing one does not remove the source or other instances.
See [prepared static GLB assets](../model/README.md) for model import and teardown.
