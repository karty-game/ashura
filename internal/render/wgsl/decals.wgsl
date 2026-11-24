struct DecalFrame {
    count: vec4u,
    projectors: array<DecalProjector, 32>,
}
@group(2) @binding(0) var<uniform> decals: DecalFrame;
@group(2) @binding(1) var decalAtlas: texture_2d<f32>;
@group(2) @binding(2) var<uniform> decalView: DecalView;
@group(2) @binding(3) var<storage, read> authored: array<DecalProjector>;
@group(2) @binding(4) var<storage, read> decalTiles: array<u32>;
@group(2) @binding(5) var<storage, read> decalMasks: array<u32>;

fn blendDecal(base: vec3f, projector: DecalProjector, position: vec3f, normal: vec3f, layers: u32, dx: vec3f, dy: vec3f) -> vec3f {
    if (projector.layers.x & layers) == 0u || dot(normal, projector.depth.xyz) <= 0.0 { return base; }
    let delta = position - decalCenter(projector);
    let local = vec3f(dot(delta, projector.right.xyz), dot(delta, projector.up.xyz), dot(delta, projector.depth.xyz));
    if any(abs(local) > vec3f(0.5)) { return base; }

    let extent = projector.region.zw - projector.region.xy;
    let uv = projector.region.xy + (vec2f(local.x, -local.y) + vec2f(0.5)) * extent;
    let uvDx = vec2f(dot(dx, projector.right.xyz), -dot(dx, projector.up.xyz)) * extent;
    let uvDy = vec2f(dot(dy, projector.right.xyz), -dot(dy, projector.up.xyz)) * extent;
    let color = textureSampleGrad(decalAtlas, lightmapSampler, uv, uvDx, uvDy);
    return mix(base, color.rgb * projector.tint.rgb, color.a * projector.tint.a);
}

fn blendOverflowDecals(base: vec3f, tile: u32, position: vec3f, normal: vec3f, layers: u32, dx: vec3f, dy: vec3f) -> vec3f {
    var result = base;
    let offset = tile * decalTileStride;
    for (var group = 0u; group < decalSummaryWords; group++) {
        var summary = decalTiles[offset + 1u + group];
        while summary != 0u {
            let word = group * 32u + firstTrailingBit(summary);
            summary &= summary - 1u;
            var bits = decalMasks[tile * decalMaskWordCount + word];
            while bits != 0u {
                let rank = word * 32u + firstTrailingBit(bits);
                bits &= bits - 1u;
                let slot = decalMasks[decalOrderOffset + rank] >> decalSlotBits;
                result = blendDecal(result, authored[slot], position, normal, layers, dx, dy);
            }
        }
    }
    return result;
}

fn projectDecals(base: vec3f, position: vec3f, normal: vec3f, layers: u32, pixel: vec2f) -> vec3f {
    let dx = dpdx(position);
    let dy = dpdy(position);
    var result = base;
    if layers == 0u { return result; }

    if decalView.viewport.z > 0.0 {
        let tile = min(vec2u(pixel * f32(decalTileAxis) / decalView.viewport.xy), vec2u(decalTileAxis - 1u));
        let tileIndex = tile.y * decalTileAxis + tile.x;
        let offset = tileIndex * decalTileStride;
        let count = decalTiles[offset];
        if count == decalTileOverflow {
            result = blendOverflowDecals(result, tileIndex, position, normal, layers, dx, dy);
        } else {
            for (var index = 0u; index < count; index++) {
                let projector = authored[decalTiles[offset + 1u + index]];
                result = blendDecal(result, projector, position, normal, layers, dx, dy);
            }
        }
    }

    for (var index = 0u; index < decals.count.x; index++) {
        result = blendDecal(result, decals.projectors[index], position, normal, layers, dx, dy);
    }

    return result;
}
