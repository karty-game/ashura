
const clusterSlices = 24u;
const clusterLightLimit = 240u;
const clusterWordBits = 32u;
const clusterMaskWords = (clusterLightLimit + clusterWordBits - 1u) / clusterWordBits;

fn clusterWords(count: u32) -> u32 {
    return (count + clusterWordBits - 1u) / clusterWordBits;
}

struct ClusterLight {
    direction: vec4f, // world position for local lights, with the kind in w
    color: vec4f,     // linear RGB and intensity
    ground: vec4f,    // ambient ground RGB or local range, point flag, spot slot, inner cosine
    spot: vec4f,      // unit spot direction and outer cosine
}

struct ClusterFrame {
    projection: mat4x4f,
    depthPlane: vec4f,
    grid: vec4f,      // viewport width, height, tile size, light count
    depth: vec4f,     // near, log2(far/near), all-lights flag
    lights: array<ClusterLight, clusterLightLimit>,
    spotMatrices: array<mat4x4f, 2>,
}

fn clusterDimensions(grid: vec4f) -> vec2u {
    return vec2u(ceil(grid.xy / grid.z));
}

fn clusterIndex(cell: vec3u, dimensions: vec2u) -> u32 {
    return (cell.z * dimensions.y + cell.y) * dimensions.x + cell.x;
}
