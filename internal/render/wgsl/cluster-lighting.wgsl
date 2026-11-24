
@group(2) @binding(0) var<uniform> clusterFrame: ClusterFrame;
@group(2) @binding(1) var<storage, read> lightMasks: array<u32>;
@group(2) @binding(2) var spotMap0: texture_depth_2d;
@group(2) @binding(3) var spotMap1: texture_depth_2d;

fn lightWord(base: u32, word: u32, count: u32, allLights: bool) -> u32 {
    if !allLights { return lightMasks[base + word]; }

    let bits = min(count - word * clusterWordBits, clusterWordBits);
    if bits == clusterWordBits { return 0xffffffffu; }
    return (1u << bits) - 1u;
}

fn fragmentClusterBase(pixel: vec2f, world: vec3f) -> u32 {
    let dimensions = clusterDimensions(clusterFrame.grid);
    let xy = min(vec2u(pixel / clusterFrame.grid.z), dimensions - vec2u(1u));
    let depth = max(clusterFrame.depth.x, dot(clusterFrame.depthPlane, vec4f(world, 1.0)));
    let slice = min(u32(log2(depth / clusterFrame.depth.x) * f32(clusterSlices) / clusterFrame.depth.y), clusterSlices - 1u);
    return clusterIndex(vec3u(xy, slice), dimensions) * clusterMaskWords;
}

fn spotlightVisibility(slot: u32, point: vec3f, towardLight: vec3f) -> f32 {
    let projected = shadowCoordinate(clusterFrame.spotMatrices[slot], point + towardLight * 0.015);
    if !shadowCoordinateValid(projected) { return 1.0; }

    if slot == 0u {
        return textureSampleCompareLevel(spotMap0, shadowSampler, projected.xy, projected.z);
    }
    return textureSampleCompareLevel(spotMap1, shadowSampler, projected.xy, projected.z);
}

fn localAttenuation(light: ClusterLight, point: vec3f, towardLight: vec3f, falloff: f32) -> f32 {
    if falloff <= 0.0 { return 0.0; }
    if light.direction.w == 3.0 {
        if light.ground.y > 0.0 { return falloff * pointVisibility(point); }
        return falloff;
    }

    let angular = smoothstep(light.spot.w, light.ground.w, dot(light.spot.xyz, -towardLight));
    if angular <= 0.0 || light.ground.z == 0.0 { return falloff * angular; }
    return falloff * angular * spotlightVisibility(u32(light.ground.z) - 1u, point, towardLight);
}
