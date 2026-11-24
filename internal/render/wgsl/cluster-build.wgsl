
@group(0) @binding(0) var<uniform> clusterFrame: ClusterFrame;
@group(0) @binding(1) var<storage, read_write> lightMasks: array<u32>;

const clusterLanes = 32u;
var<workgroup> lightDepths: array<vec2f, clusterLightLimit>;

fn matrixRow(matrix: mat4x4f, row: u32) -> vec4f {
    return vec4f(matrix[0][row], matrix[1][row], matrix[2][row], matrix[3][row]);
}

fn sphereInside(plane: vec4f, center: vec4f, radius: f32) -> bool {
    // Include lights on either side of a tile boundary within float precision.
    let allowance = 1e-5 * (dot(abs(plane), abs(center)) + radius + 1.0);
    return dot(plane, center) + radius * length(plane.xyz) + allowance >= 0.0;
}

fn tileLightDepth(light: ClusterLight, left: vec4f, right: vec4f, bottom: vec4f, top: vec4f) -> vec2f {
    if light.color.w == 0.0 { return vec2f(0.0, -1.0); }
    if light.direction.w < 3.0 { return vec2f(0.0, 1e38); }

    let center = vec4f(light.direction.xyz, 1.0);
    let radius = light.ground.x;
    if !sphereInside(left, center, radius) || !sphereInside(right, center, radius) ||
       !sphereInside(bottom, center, radius) || !sphereInside(top, center, radius) {
        return vec2f(0.0, -1.0);
    }

    let plane = clusterFrame.depthPlane;
    let depth = dot(plane, center);
    let allowance = 1e-5 * (dot(abs(plane), abs(center)) + radius + abs(depth) + 1.0);
    let reach = radius * length(plane.xyz) + allowance;
    return vec2f(depth - reach, depth + reach);
}

@compute @workgroup_size(clusterLanes)
fn buildLightMasks(@builtin(workgroup_id) tile: vec3u, @builtin(local_invocation_index) lane: u32) {
    let dimensions = clusterDimensions(clusterFrame.grid);
    let pixelMin = vec2f(tile.xy) * clusterFrame.grid.z;
    let pixelMax = min(pixelMin + clusterFrame.grid.z, clusterFrame.grid.xy);
    let ndcMin = vec2f(pixelMin.x, pixelMax.y) / clusterFrame.grid.xy * vec2f(2.0, -2.0) + vec2f(-1.0, 1.0);
    let ndcMax = vec2f(pixelMax.x, pixelMin.y) / clusterFrame.grid.xy * vec2f(2.0, -2.0) + vec2f(-1.0, 1.0);
    let x = matrixRow(clusterFrame.projection, 0u);
    let y = matrixRow(clusterFrame.projection, 1u);
    let w = matrixRow(clusterFrame.projection, 3u);
    let left = x - ndcMin.x * w;
    let right = ndcMax.x * w - x;
    let bottom = y - ndcMin.y * w;
    let top = ndcMax.y * w - y;

    // A light's screen-tile test is shared by all 24 logarithmic depth slices.
    let count = u32(clusterFrame.grid.w);
    for (var index = lane; index < count; index += clusterLanes) {
        lightDepths[index] = tileLightDepth(clusterFrame.lights[index], left, right, bottom, top);
    }
    workgroupBarrier();
    if lane >= clusterSlices { return; }

    let slices = clusterFrame.depth.x * exp2(vec2f(f32(lane), f32(lane + 1u)) * (clusterFrame.depth.y / f32(clusterSlices)));
    let base = clusterIndex(vec3u(tile.xy, lane), dimensions) * clusterMaskWords;
    for (var word = 0u; word < clusterWords(count); word++) {
        let first = word * clusterWordBits;
        var mask = 0u;
        for (var index = first; index < min(first + clusterWordBits, count); index++) {
            let depths = lightDepths[index];
            if depths.x <= slices.y && depths.y >= slices.x {
                mask |= 1u << (index - first);
            }
        }
        lightMasks[base + word] = mask;
    }
}
