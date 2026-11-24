struct DecalProjector {
    right: vec4f,
    up: vec4f,
    depth: vec4f,
    region: vec4f,
    tint: vec4f,
    layers: vec4u,
}
struct DecalView {
    projection: mat4x4f,
    viewport: vec4f, // width, height, authored slot high-water mark, reserved
}
const decalTileAxis = 16u;
const decalTileCount = decalTileAxis * decalTileAxis;
const decalTileLimit = 64u;
const decalTileStride = decalTileLimit + 1u;
const decalTileOverflow = decalTileLimit + 1u;
const decalSlotBits = 13u;
const decalSlotMask = (1u << decalSlotBits) - 1u;
const decalMaskWordCount = (decalSlotMask + 1u) / 32u;
const decalOrderOffset = decalTileCount * decalMaskWordCount;
const decalSummaryWords = decalMaskWordCount / 32u;

fn decalCenter(projector: DecalProjector) -> vec3f {
    return vec3f(projector.right.w, projector.up.w, projector.depth.w);
}
