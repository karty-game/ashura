struct Parameters {
    positionRange: vec4f,
    colorScale: vec4f,
    dimensions: vec4u,
    settings: vec4f,
}
struct Node {
    minimum: vec3f,
    first: u32,
    maximum: vec3f,
    count: u32,
}
struct Triangle { a: vec4f, b: vec4f, c: vec4f }
struct Receiver { positionCoverage: vec4f, normalChart: vec4f }
@group(0) @binding(0) var<uniform> parameters: Parameters;
@group(0) @binding(1) var<storage, read> nodes: array<Node>;
@group(0) @binding(2) var<storage, read> triangles: array<Triangle>;
@group(0) @binding(3) var<storage, read> receivers: array<Receiver>;
@group(0) @binding(4) var<storage, read> padding: array<u32>;
@group(0) @binding(5) var outputMap: texture_storage_2d<rgba8unorm, write>;

fn intersectsBox(origin: vec3f, direction: vec3f, distance: f32, node: Node) -> bool {
    var near = 0.0;
    var far = distance;
    for (var axis = 0u; axis < 3u; axis++) {
        if abs(direction[axis]) < 1e-12 {
            if origin[axis] < node.minimum[axis] || origin[axis] > node.maximum[axis] { return false; }
        } else {
            let a = (node.minimum[axis] - origin[axis]) / direction[axis];
            let b = (node.maximum[axis] - origin[axis]) / direction[axis];
            near = max(near, min(a, b));
            far = min(far, max(a, b));
            if near > far { return false; }
        }
    }
    return true;
}

fn intersectsTriangle(origin: vec3f, direction: vec3f, distance: f32, triangle: Triangle) -> bool {
    let first = triangle.b.xyz - triangle.a.xyz;
    let second = triangle.c.xyz - triangle.a.xyz;
    let crossDirection = cross(direction, second);
    let determinant = dot(first, crossDirection);
    if abs(determinant) < 1e-9 { return false; }
    let delta = origin - triangle.a.xyz;
    let u = dot(delta, crossDirection) / determinant;
    if u < -1e-6 || u > 1.000001 { return false; }
    let crossDelta = cross(delta, first);
    let v = dot(direction, crossDelta) / determinant;
    if v < -1e-6 || u + v > 1.000001 { return false; }
    let hit = dot(second, crossDelta) / determinant;
    return hit > 0.0001 && hit < distance;
}

fn blocked(origin: vec3f, direction: vec3f, distance: f32) -> bool {
    var stack: array<u32, 64>;
    var length = 1u;
    stack[0] = 0u;
    loop {
        if length == 0u { break; }
        length--;
        let node = nodes[stack[length]];
        if !intersectsBox(origin, direction, distance, node) { continue; }
        if node.count > 0u {
            for (var i = 0u; i < node.count; i++) {
                if intersectsTriangle(origin, direction, distance, triangles[node.first + i]) { return true; }
            }
        } else {
            // Complete admission proves tree depth and adjacent child references.
            stack[length] = node.first;
            stack[length + 1u] = node.first + 1u;
            length += 2u;
        }
    }
    return false;
}

@compute @workgroup_size(8, 8, 1)
fn bake_direct(@builtin(global_invocation_id) invocation: vec3u) {
    let size = parameters.dimensions.xy;
    if invocation.x >= size.x || invocation.y >= size.y { return; }
    let pixel = invocation.y * size.x + invocation.x;
    let source = padding[pixel];
    if source == 0xffffffffu {
        textureStore(outputMap, vec2i(invocation.xy), vec4f(0.0));
        return;
    }
    // Padding references a chart-local covered texel prepared by the build.
    // Shade it directly, requiring no runtime dilation pass or CPU readback.
    let receiver = receivers[source];
    let delta = parameters.positionRange.xyz - receiver.positionCoverage.xyz;
    let squaredDistance = dot(delta, delta);
    let range = parameters.positionRange.w;
    var irradiance = vec3f(0.0);
    if squaredDistance > 1e-12 && squaredDistance < range * range {
        let distance = sqrt(squaredDistance);
        let direction = delta / distance;
        let cosine = dot(receiver.normalChart.xyz, direction);
        let diffuse = select(max(cosine, 0.0), abs(cosine), receiver.positionCoverage.w > 1.5);
        let bias = min(parameters.settings.x, distance * 0.5);
        if diffuse > 0.0 && !blocked(receiver.positionCoverage.xyz + direction * bias, direction, distance - bias) {
            let falloff = 1.0 - squaredDistance / (range * range);
            irradiance = parameters.colorScale.xyz * (diffuse * falloff * falloff) / parameters.colorScale.w;
        }
    }
    textureStore(outputMap, vec2i(invocation.xy), vec4f(irradiance, 1.0));
}
