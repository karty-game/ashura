struct PointShadow {
    positionRange: vec4f,
    settings: vec4f,
    faces: array<mat4x4f, 6>,
}
@group(1) @binding(0) var<uniform> pointShadow: PointShadow;
@group(1) @binding(1) var shadowSampler: sampler_comparison;
@group(1) @binding(2) var pointShadowMaps: texture_depth_2d_array;

fn shadowCoordinate(matrix: mat4x4f, receiver: vec3f) -> vec3f {
    let clip = matrix * vec4f(receiver, 1.0);
    if clip.w <= 0.0 { return vec3f(-1.0); }
    let ndc = clip.xyz / clip.w;
    return vec3f(ndc.xy * vec2f(0.5, -0.5) + vec2f(0.5), ndc.z);
}

fn shadowCoordinateValid(projected: vec3f) -> bool {
    return all(projected >= vec3f(0.0)) && all(projected <= vec3f(1.0));
}

fn pointShadowFace(offset: vec3f) -> u32 {
    let magnitude = abs(offset);
    if magnitude.x >= max(magnitude.y, magnitude.z) { return select(1u, 0u, offset.x >= 0.0); }
    if magnitude.y >= magnitude.z { return select(3u, 2u, offset.y >= 0.0); }
    return select(5u, 4u, offset.z >= 0.0);
}

fn pointVisibility(point: vec3f) -> f32 {
    if pointShadow.settings.y == 0.0 { return 1.0; }
    let offset = point - pointShadow.positionRange.xyz;
    let distance = length(offset);
    if distance <= 1e-5 || distance >= pointShadow.positionRange.w { return 1.0; }
    let face = pointShadowFace(offset);
    let receiver = point - offset * (pointShadow.settings.x / distance);
    let projected = shadowCoordinate(pointShadow.faces[face], receiver);
    if !shadowCoordinateValid(projected) { return 1.0; }
    return textureSampleCompareLevel(pointShadowMaps, shadowSampler, projected.xy, i32(face), projected.z);
}
