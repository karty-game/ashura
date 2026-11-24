@group(0) @binding(0) var<uniform> decalView: DecalView;
@group(0) @binding(1) var<storage, read> authored: array<DecalProjector>;
@group(0) @binding(2) var<storage, read_write> masks: array<atomic<u32>>;
@group(0) @binding(3) var<storage, read_write> tiles: array<atomic<u32>>;

@compute @workgroup_size(64)
fn clearMasks(@builtin(global_invocation_id) id: vec3u) {
    atomicStore(&masks[id.x], 0u);
    // Start each tile's nonempty-word summary in the short-list storage.
    if id.x < decalTileCount * decalSummaryWords {
        let tile = id.x / decalSummaryWords;
        let group = id.x % decalSummaryWords;
        atomicStore(&tiles[tile * decalTileStride + 1u + group], 0u);
    }
}

fn clipRadii(matrix: mat4x4f, radius: f32) -> vec4f {
    let xyz0 = matrix[0] * radius;
    let xyz1 = matrix[1] * radius;
    let xyz2 = matrix[2] * radius;
    return sqrt(xyz0 * xyz0 + xyz1 * xyz1 + xyz2 * xyz2);
}

fn tileCoordinate(ndc: vec2f) -> vec2u {
    let uv = clamp(ndc * vec2f(0.5, -0.5) + vec2f(0.5), vec2f(0.0), vec2f(1.0));
    return min(vec2u(uv * f32(decalTileAxis)), vec2u(decalTileAxis - 1u));
}

@compute @workgroup_size(64)
fn binProjectors(@builtin(global_invocation_id) id: vec3u) {
    let index = id.x;
    if index >= u32(decalView.viewport.z) { return; }
    let projector = authored[index];
    if projector.layers.x == 0u || projector.tint.a == 0.0 { return; }

    let center = decalView.projection * vec4f(decalCenter(projector), 1.0);
    let r = clipRadii(decalView.projection, bitcast<f32>(projector.layers.y));
    let pad = 1e-5 * (abs(center) + r + vec4f(1.0));
    let lower = center - r - pad;
    let upper = center + r + pad;
    if upper.w <= 0.0 || upper.z < 0.0 || lower.z > upper.w ||
       lower.x > upper.w || upper.x < -upper.w || lower.y > upper.w || upper.y < -upper.w { return; }

    var first = vec2u(0u);
    var last = vec2u(decalTileAxis - 1u);
    if lower.w > 0.0 {
        // A sphere's projected extents are contained by these clip-space
        // intervals, including objects crossing a side or depth boundary.
        let lo = min(lower.xy / lower.w, lower.xy / upper.w);
        let hi = max(upper.xy / lower.w, upper.xy / upper.w);
        first = tileCoordinate(vec2f(lo.x, hi.y));
        last = tileCoordinate(vec2f(hi.x, lo.y));
    }
    let rank = atomicLoad(&masks[decalOrderOffset + index]) & decalSlotMask;
    let word = rank / 32u;
    let mask = 1u << (rank % 32u);
    for (var y = first.y; y <= last.y; y++) {
        for (var x = first.x; x <= last.x; x++) {
            let tile = y * decalTileAxis + x;
            let previous = atomicOr(&masks[tile * decalMaskWordCount + word], mask);
            if previous == 0u {
                atomicOr(&tiles[tile * decalTileStride + 1u + word / 32u], 1u << (word % 32u));
            }
        }
    }
}

@compute @workgroup_size(1)
fn compactTiles(@builtin(global_invocation_id) id: vec3u) {
    let tile = id.x;
    let base = tile * decalTileStride;
    var count = 0u;
    var selected: array<u32, decalTileLimit>;
    for (var group = 0u; group < decalSummaryWords; group++) {
        var summary = atomicLoad(&tiles[base + 1u + group]);
        while summary != 0u {
            let word = group * 32u + firstTrailingBit(summary);
            summary &= summary - 1u;
            var maskWord = atomicLoad(&masks[tile * decalMaskWordCount + word]);
            if count + countOneBits(maskWord) > decalTileLimit {
                // The fragment shader consumes the original mask and summary.
                atomicStore(&tiles[base], decalTileOverflow);
                return;
            }

            // Mask bits already follow blend order.
            while maskWord != 0u {
                let rank = word * 32u + firstTrailingBit(maskWord);
                maskWord &= maskWord - 1u;
                selected[count] = atomicLoad(&masks[decalOrderOffset + rank]) >> decalSlotBits;
                count++;
            }
        }
    }
    atomicStore(&tiles[base], count);
    for (var index = 0u; index < count; index++) {
        atomicStore(&tiles[base + 1u + index], selected[index]);
    }
}
