struct Light {
    direction: vec4f,
    color: vec4f,
    ground: vec4f,
}
struct Material {
    viewProjection: mat4x4f,
    camera: vec4f,
    tint: vec4f,
    parameters: vec4f,
    occlusion: vec4f,
    probeOrigin: vec4f,
    probeMinimum: vec4f,
    probeMaximum: vec4f,
    lights: array<Light, 8>,
}
struct Capture {
    right: vec4f,
    up: vec4f,
    outward: vec4f,
}
struct Impostor {
    centerRadius: vec4f,
    atlas: vec4f,
    grid: vec4f,
    settings: vec4f,
    bounds: vec4f,
    encoding: vec4f,
    views: array<Capture, 64>,
}
@group(0) @binding(0) var<uniform> material: Material;
@group(0) @binding(1) var surfaceSampler: sampler;
@group(0) @binding(2) var colorMap: texture_2d<f32>;
@group(0) @binding(3) var normalDepthMap: texture_2d<f32>;
@group(0) @binding(4) var<uniform> impostor: Impostor;
struct VertexOutput {
    @builtin(position) position: vec4f,
    @location(0) objectPoint: vec3f,
    @location(1) @interpolate(flat, either) objectCamera: vec3f,
    @location(2) @interpolate(flat, either) model0: vec4f,
    @location(3) @interpolate(flat, either) model1: vec4f,
    @location(4) @interpolate(flat, either) model2: vec4f,
    @location(5) @interpolate(flat, either) model3: vec4f,
    @location(6) @interpolate(flat, either) selected: vec4u,
    @location(7) @interpolate(flat, either) weights: vec4f,
}
fn safeNormalize(value: vec3f) -> vec3f {
    return value * inverseSqrt(max(dot(value, value), 1e-12));
}
fn inverseTranspose(model: mat4x4f) -> mat3x3f {
    let first = cross(model[1].xyz, model[2].xyz);
    let second = cross(model[2].xyz, model[0].xyz);
    let third = cross(model[0].xyz, model[1].xyz);
    let determinant = dot(model[0].xyz, first);
    let divisor = select(determinant, 1.0, abs(determinant) < 1e-12);
    return mat3x3f(first / divisor, second / divisor, third / divisor);
}
fn objectPosition(model: mat4x4f, world: vec3f) -> vec3f {
    return transpose(inverseTranspose(model)) * (world - model[3].xyz);
}
@vertex
fn vs_main(
    @location(0) position: vec3f,
    @location(1) normal: vec3f,
    @location(2) uv: vec2f,
    @location(3) model0: vec4f,
    @location(4) model1: vec4f,
    @location(5) model2: vec4f,
    @location(6) model3: vec4f,
) -> VertexOutput {
    let model = mat4x4f(model0, model1, model2, model3);
    let objectCamera = objectPosition(model, material.camera.xyz);
    let direction = safeNormalize(objectCamera - impostor.centerRadius.xyz);
    let mode = u32(impostor.bounds.w);
    var selected = vec4u(0u);
    var weights = vec4f(0.0);
    let worldCenter = (model * vec4f(impostor.centerRadius.xyz, 1.0)).xyz;
    let cameraDirection = safeNormalize(material.camera.xyz - worldCenter);
    var referenceUp = vec3f(0.0, 1.0, 0.0);
    if abs(cameraDirection.y) > 0.999 { referenceUp = vec3f(0.0, 0.0, 1.0); }
    let right = safeNormalize(cross(referenceUp, cameraDirection));
    let up = safeNormalize(cross(cameraDirection, right));
    let worldRadius = impostor.centerRadius.w * max(length(model0.xyz), max(length(model1.xyz), length(model2.xyz)));
    var radiusSpan = worldRadius;
    if mode == 5u || mode == 4u {
        let cameraDelta = material.camera.xyz - worldCenter;
        let distanceSquared = dot(cameraDelta, cameraDelta);
        if distanceSquared > worldRadius * worldRadius * 1.000001 {
            radiusSpan = worldRadius * sqrt(distanceSquared / (distanceSquared - worldRadius * worldRadius));
        } else { weights.w = 1.0; }
    }
    let worldPoint = worldCenter + (right * position.x + up * position.y) * radiusSpan;
    if mode == 4u {
        let grid = u32(impostor.grid.z);
        let projected = direction / max(dot(abs(direction), vec3f(1.0)), 1e-8);
        var octahedral = projected.xz;
        if projected.y < 0.0 {
            let signs = select(vec2f(-1.0), vec2f(1.0), octahedral >= vec2f(0.0));
            octahedral = (vec2f(1.0) - abs(octahedral.yx)) * signs;
        }
        let coordinate = clamp(octahedral * 0.5 + vec2f(0.5), vec2f(0.0), vec2f(1.0)) * f32(grid - 1u);
        let cell = min(vec2u(floor(coordinate)), vec2u(grid - 2u));
        let fraction = coordinate - vec2f(cell);
        let first = cell.y * grid + cell.x;
        // Follow the octahedron face diagonal in each quadrant. The mirrored
        // quadrants have the opposite diagonal; a fixed diagonal interpolates
        // across a folded face and makes view handovers change projection.
        if projected.x * projected.z < 0.0 {
            if fraction.x >= fraction.y {
                selected = vec4u(first, first + 1u, first + grid + 1u, 0u);
                weights = vec4f(1.0 - fraction.x, fraction.x - fraction.y, fraction.y, 0.0);
            } else {
                selected = vec4u(first, first + grid + 1u, first + grid, 0u);
                weights = vec4f(1.0 - fraction.y, fraction.x, fraction.y - fraction.x, 0.0);
            }
        } else if fraction.x + fraction.y <= 1.0 {
            selected = vec4u(first, first + 1u, first + grid, 0u);
            weights = vec4f(1.0 - fraction.x - fraction.y, fraction.x, fraction.y, 0.0);
        } else {
            selected = vec4u(first + grid + 1u, first + grid, first + 1u, 0u);
            weights = vec4f(fraction.x + fraction.y - 1.0, 1.0 - fraction.x, 1.0 - fraction.y, 0.0);
        }
    } else if mode == 0u {
        let azimuths = u32(impostor.grid.z);
        let elevations = u32(impostor.grid.w);
        let angle = atan2(direction.x, direction.z);
        let azimuth = fract(angle / 6.28318530718 + 1.0) * f32(azimuths);
        let az0 = u32(floor(azimuth)) % azimuths;
        let az1 = (az0 + 1u) % azimuths;
        let elevation = clamp((asin(clamp(direction.y, -1.0, 1.0)) - impostor.settings.x) /
            (impostor.settings.y - impostor.settings.x), 0.0, 1.0) * f32(elevations - 1u);
        let el0 = min(u32(floor(elevation)), elevations - 2u);
        let el1 = el0 + 1u;
        let azWeight = fract(azimuth);
        let elWeight = clamp(elevation - f32(el0), 0.0, 1.0);
        selected = vec4u(el0 * azimuths + az0, el0 * azimuths + az1, el1 * azimuths + az0, el1 * azimuths + az1);
        let linearWeights = vec4f((1.0-azWeight)*(1.0-elWeight), azWeight*(1.0-elWeight), (1.0-azWeight)*elWeight, azWeight*elWeight);
        let squaredWeights = linearWeights * linearWeights;
        let sharpWeights = squaredWeights * squaredWeights;
        weights = sharpWeights / max(dot(sharpWeights, vec4f(1.0)), 1e-8);
    }
    var output: VertexOutput;
    output.position = material.viewProjection * vec4f(worldPoint, 1.0);
    output.objectPoint = objectPosition(model, worldPoint);
    output.objectCamera = objectCamera;
    output.model0 = model0;
    output.model1 = model1;
    output.model2 = model2;
    output.model3 = model3;
    output.selected = selected;
    output.weights = weights;
    return output;
}
fn captureUV(point: vec3f, capture: Capture) -> vec2f {
    let local = (point - impostor.centerRadius.xyz) / impostor.centerRadius.w;
    return vec2f(dot(local, capture.right.xyz) * 0.5 + 0.5, 0.5 - dot(local, capture.up.xyz) * 0.5);
}
fn atlasUV(viewIndex: u32, local: vec2f) -> vec2f {
    let columns = u32(impostor.grid.x);
    let cell = vec2f(f32(viewIndex % columns), f32(viewIndex / columns));
    let halfTexel = 0.5 / impostor.atlas.w;
    let coordinate = clamp(local, vec2f(halfTexel), vec2f(1.0-halfTexel));
    return (cell * impostor.atlas.z + vec2f(impostor.grid.y) + coordinate * impostor.atlas.w) / impostor.atlas.xy;
}
struct Surface {
    color: vec3f,
    normal: vec3f,
    point: vec3f,
    coverage: f32,
    traceFailed: bool,
}
fn captureDepth(value: vec4f) -> f32 {
    return select(value.a, value.g, impostor.encoding.x == 1.0);
}
fn captureNormal(value: vec4f) -> vec3f {
    if impostor.encoding.x == 1.0 { return layerNormal(value.rb); }
    return value.rgb * 2.0 - vec3f(1.0);
}
fn captureColor(value: vec4f) -> vec3f {
    // BC1's transparent selector is black. Filtering therefore premultiplies
    // RGB by coverage, unlike the dilated straight-alpha RGBA8 captures.
    if impostor.encoding.x == 1.0 { return value.rgb / max(value.a, 1e-6); }
    return value.rgb;
}
fn sampleAngularSurface(input: VertexOutput) -> Surface {
    let toCamera = safeNormalize(input.objectCamera - input.objectPoint);
    let footprint = max(length(dpdx(input.objectPoint)), length(dpdy(input.objectPoint))) * impostor.atlas.w /
        (2.0 * impostor.centerRadius.w);
    let lod = clamp(log2(max(footprint, 1e-6)), 0.0, f32(textureNumLevels(colorMap)-1u));
    var result: Surface;
    result.color = vec3f(0.0);
    result.normal = vec3f(0.0);
    result.point = vec3f(0.0);
    result.coverage = 0.0;
    for (var index = 0u; index < 4u; index++) {
        let viewIndex = input.selected[index];
        let capture = impostor.views[viewIndex];
        let initial = captureUV(input.objectPoint, capture);
        // Keep color and normals at the same capture coordinate. Warping color
        // alone from a single depth fetch creates disocclusion noise on dense
        // silhouettes; captured depth still corrects the fragment depth below.
        // Exactly one normal/depth fetch and one color/coverage fetch per view.
        let normalDepth = textureSampleLevel(normalDepthMap, surfaceSampler, atlasUV(viewIndex, initial), lod);
        let depth = (captureDepth(normalDepth) * 2.0 - 1.0) * impostor.centerRadius.w;
        let denominator = dot(toCamera, capture.outward.xyz);
        let signedDenominator = select(-1.0, 1.0, denominator >= 0.0) * max(abs(denominator), 0.1);
        let distance = (depth - dot(input.objectPoint-impostor.centerRadius.xyz, capture.outward.xyz)) / signedDenominator;
        let point = input.objectPoint + toCamera * clamp(distance, -2.0*impostor.centerRadius.w, 2.0*impostor.centerRadius.w);
        let color = textureSampleLevel(colorMap, surfaceSampler, atlasUV(viewIndex, initial), lod);
        let inside = all(initial >= vec2f(0.0)) && all(initial <= vec2f(1.0));
        let weight = input.weights[index] * color.a * select(0.0, 1.0, inside);
        result.color += captureColor(color) * weight;
        result.normal += captureNormal(normalDepth) * weight;
        result.point += point * weight;
        result.coverage += weight;
    }
    let inverseWeight = 1.0 / max(result.coverage, 1e-6);
    result.color *= inverseWeight;
    result.normal = safeNormalize(result.normal);
    result.point *= inverseWeight;
    return result;
}
// Quality uses the projected sphere diameter, without CPU distance scans.
// Negative step encodings select a fixed 0/1/2-step reference. Zeroed legacy
// providers retain one correction. Threshold ramps keep UVs continuous.
fn correctionAmounts(input: VertexOutput) -> vec2f {
    let quality = material.parameters;
    if quality.z < 0.0 {
        let count = clamp(-quality.z - 1.0, 0.0, 2.0);
        return clamp(vec2f(count, count - 1.0), vec2f(0.0), vec2f(1.0));
    }
    if quality.x <= 0.0 || quality.y < quality.x * 2.0 { return vec2f(1.0, 0.0); }
    let footprint = max(length(dpdx(input.objectPoint)), length(dpdy(input.objectPoint)));
    let pixels = 2.0 * impostor.centerRadius.w / max(footprint, 1e-8);
    let count = clamp(quality.z, 0.0, 2.0);
    return vec2f(smoothstep(quality.x, quality.x * 2.0, pixels),
        smoothstep(quality.y, quality.y * 2.0, pixels)) *
        clamp(vec2f(count, count - 1.0), vec2f(0.0), vec2f(1.0));
}
// Three views always blend. Each view has two final attribute reads and at most
// two dependent correction reads: six/nine/twelve reads, without volume tracing.
fn sampleOctahedralSurface(input: VertexOutput) -> Surface {
    let ray = safeNormalize(input.objectPoint - input.objectCamera);
    let cameraOffset = input.objectCamera - impostor.centerRadius.xyz;
    let radius = impostor.centerRadius.w;
    let corrections = correctionAmounts(input);
    var result: Surface;
    for (var index = 0u; index < 3u; index++) {
        let viewIndex = input.selected[index];
        let capture = impostor.views[viewIndex];
        let denominator = dot(ray, capture.outward.xyz);
        let divisor = -max(-denominator, 0.1);
        let cameraDepth = dot(cameraOffset, capture.outward.xyz);
        // Start halfway toward the capture camera. Seeding at the center
        // plane samples too far behind a convex silhouette and can overshoot
        // into an unrelated background texel on the single parallax step.
        let seedDistance = (radius * 0.5 - cameraDepth) / divisor;
        let seedPoint = input.objectCamera + ray * seedDistance;
        let seed = captureUV(seedPoint, capture);
        // Derivatives belong to the unwarped projection. Depth discontinuities
        // must not force coarse mips or contaminate neighboring atlas cells.
        let footprint = max(length(dpdx(seed)), length(dpdy(seed))) * impostor.atlas.w;
        let lod = clamp(log2(max(footprint, 1e-6)) + material.parameters.w,
            0.0, f32(textureNumLevels(colorMap)-1u));
        var coordinate = seed;
        var firstDistance = seedDistance;
        if corrections.x > 0.0 {
            let initial = textureSampleLevel(normalDepthMap, surfaceSampler, atlasUV(viewIndex, coordinate), lod);
            let distance = ((captureDepth(initial) * 2.0 - 1.0) * radius - cameraDepth) / divisor;
            firstDistance = distance;
            coordinate = mix(coordinate, captureUV(input.objectCamera + ray * distance, capture), corrections.x);
        }
        if corrections.y > 0.0 {
            let initial = textureSampleLevel(normalDepthMap, surfaceSampler, atlasUV(viewIndex, coordinate), lod);
            let distance = ((captureDepth(initial) * 2.0 - 1.0) * radius - cameraDepth) / divisor;
            // Refine a bracketed crossing instead of repeating a fixed-point
            // jump, which can oscillate across a silhouette. If the two depth
            // samples do not bracket a root, retain the first projection.
            let firstResidual = firstDistance - seedDistance;
            let residual = distance - firstDistance;
            if firstResidual * residual <= 0.0 && abs(firstResidual - residual) > 1e-6 * radius {
                let fraction = clamp(firstResidual / (firstResidual - residual), 0.0, 1.0);
                let refinedDistance = mix(seedDistance, firstDistance, fraction);
                coordinate = mix(coordinate, captureUV(input.objectCamera + ray * refinedDistance, capture), corrections.y);
            }
        }
        let correctedUV = atlasUV(viewIndex, coordinate);
        let normalDepth = textureSampleLevel(normalDepthMap, surfaceSampler, correctedUV, lod);
        let color = textureSampleLevel(colorMap, surfaceSampler, correctedUV, lod);
        let correctedDistance = ((captureDepth(normalDepth) * 2.0 - 1.0) * radius - cameraDepth) / divisor;
        let inside = all(coordinate >= vec2f(0.0)) && all(coordinate <= vec2f(1.0)) &&
            correctedDistance > 0.0 && denominator < -0.1;
        let weight = input.weights[index] * color.a * select(0.0, 1.0, inside);
        result.coverage += weight;
        result.color += captureColor(color) * weight;
        result.normal += captureNormal(normalDepth) * weight;
        result.point += (input.objectCamera + ray * correctedDistance) * weight;
    }
    let inverseWeight = 1.0 / max(result.coverage, 1e-6);
    result.color *= inverseWeight;
    result.normal = safeNormalize(result.normal);
    result.point *= inverseWeight;
    return result;
}
// Numeric hierarchy and interval texels are never filtered or mip averaged.
fn layerNumeric(coordinate: vec2u) -> vec4u {
    return vec4u(round(textureLoad(normalDepthMap, vec2i(coordinate), 0) * 255.0));
}
fn layerTile(tile: u32, pixel: vec2u, baseRow: u32) -> vec2u {
    return vec2u(tile % 8u, tile / 8u) * u32(impostor.atlas.z) +
        pixel + vec2u(u32(impostor.grid.y), baseRow + u32(impostor.grid.y));
}
fn layerDepth(low: u32, high: u32) -> f32 { return f32(low + 256u * high) / 65535.0; }
fn layerNormal(encoded: vec2f) -> vec3f {
    let value = encoded * 2.0 - vec2f(1.0);
    var normal = vec3f(value, 1.0 - abs(value.x) - abs(value.y));
    if normal.z < 0.0 {
        normal = vec3f((vec2f(1.0) - abs(normal.yx)) *
            select(vec2f(-1.0), vec2f(1.0), normal.xy >= vec2f(0.0)), normal.z);
    }
    return safeNormalize(normal);
}
fn layerBox(origin: vec3f, inverseDirection: vec3f, parallel: vec3<bool>, minimum: vec3f, maximum: vec3f) -> vec2f {
    var near = 0.0;
    var far = 1e20;
    for (var axis = 0u; axis < 3u; axis++) {
        if parallel[axis] {
            // Half-open XY cells give a boundary-parallel ray one owner.
            if origin[axis] < minimum[axis] || origin[axis] > maximum[axis] ||
                (axis < 2u && origin[axis] == maximum[axis] && maximum[axis] < 1.0) {
                return vec2f(1e20, -1.0);
            }
        } else {
            let first = (minimum[axis] - origin[axis]) * inverseDirection[axis];
            let second = (maximum[axis] - origin[axis]) * inverseDirection[axis];
            near = max(near, min(first, second));
            far = min(far, max(first, second));
        }
    }
    return vec2f(near, far);
}
fn layerAttributes(point: vec3f, traceAxis: u32, tracePixel: vec2u) -> Surface {
    var result: Surface;
    result.point = point;
    let resolution = u32(impostor.atlas.w);
    let slots = 2u * u32(impostor.grid.z);
    let local = (point - impostor.centerRadius.xyz) / impostor.centerRadius.w;
    let scale = 2.0 * impostor.centerRadius.w;
    var closest = 1e20;
    var selected = vec2u(0u);
    var selectedNormal = vec2f(0.0);
    for (var axis = 0u; axis < 3u; axis++) {
        let capture = impostor.views[axis];
        let uv = vec2f(0.5 + dot(local, capture.right.xyz) * 0.5, 0.5 - dot(local, capture.up.xyz) * 0.5);
        var pixel = min(vec2u(clamp(uv, vec2f(0.0), vec2f(1.0)) * f32(resolution)), vec2u(resolution - 1u));
        if axis == traceAxis { pixel = tracePixel; }
        let depth = 0.5 + dot(local, capture.outward.xyz) * 0.5;
        let offset = ((vec2f(pixel) + vec2f(0.5)) / f32(resolution) - uv) * scale;
        let transverseError = dot(offset, offset);
        if transverseError >= closest { continue; }
        var low = 0u;
        var high = slots - 1u;
        for (var step = 0u; step < 5u; step++) {
            if high - low <= 1u { break; }
            let middle = (low + high) / 2u;
            let value = layerNumeric(layerTile(axis * slots + middle, pixel, 0u));
            if layerDepth(value.b, value.a) > depth { low = middle; } else { high = middle; }
        }
        for (var neighbor = 0u; neighbor < 2u; neighbor++) {
            let slot = select(low, high, neighbor == 1u);
            let coordinate = layerTile(axis * slots + slot, pixel, 0u);
            let normalDepth = textureLoad(normalDepthMap, vec2i(coordinate), 0);
            let bytes = vec4u(round(normalDepth * 255.0));
            if bytes.b + bytes.a == 0u { continue; }
            let delta = (layerDepth(bytes.b, bytes.a) - depth) * scale;
            let error = delta * delta + transverseError;
            if error < closest {
                closest = error;
                selected = coordinate;
                selectedNormal = normalDepth.rg;
            }
        }
    }
    let color = textureLoad(colorMap, vec2i(selected), 0);
    result.color = color.rgb;
    result.coverage = color.a;
    if closest < 1e20 { result.normal = layerNormal(selectedNormal); }
    return result;
}
// Compare boundary TIMES rather than rounded positions; never epsilon-step the ray.
fn layerOwner(origin: vec2f, direction: vec2f, inverse: vec2f, parallel: vec2<bool>,
    time: f32, resolution: u32, forward: bool) -> vec2i {
    let position = select(origin + direction * time, origin, parallel);
    let boundary = round(position * f32(resolution));
    let crossing = (boundary / f32(resolution) - origin) * inverse;
    let before = select(vec2f(time) <= crossing, vec2f(time) < crossing, vec2<bool>(forward));
    let subtract = select(before, !before, direction < vec2f(0.0));
    return select(vec2i(boundary) - select(vec2i(0), vec2i(1), subtract),
        clamp(vec2i(floor(origin * f32(resolution))), vec2i(0), vec2i(i32(resolution) - 1)), parallel);
}
// Closed corner contacts need incident leaf point queries, not a traversal stack.
// Return pixel.xy, status (0 miss/1 hit/2 budget), and charged node visits.
fn layerContainsZ(time: f32, origin: f32, inverse: f32, parallel: bool, minimum: f32, maximum: f32) -> bool {
    if parallel { return origin >= minimum && origin <= maximum; }
    let first = (minimum - origin) * inverse;
    let second = (maximum - origin) * inverse;
    return time >= min(first, second) && time <= max(first, second);
}
fn layerPoint(origin: vec3f, inverseZ: f32, parallelZ: bool, time: f32, first: vec2i, second: vec2i,
    resolution: u32, intervals: u32, axis: u32, hierarchy: u32, pairBase: u32,
    width: u32, remaining: u32) -> vec4u {
    var used = 0u;
    let dimensions = select(vec2u(1u), vec2u(2u), first != second);
    for (var y = 0u; y < dimensions.y; y++) {
        for (var x = 0u; x < dimensions.x; x++) {
            let pixel = vec2i(select(first.x, second.x, x == 1u), select(first.y, second.y, y == 1u));
            if any(pixel < vec2i(0)) || any(pixel >= vec2i(i32(resolution))) { continue; }
            if used >= remaining { return vec4u(0u, 0u, 2u, used); }
            used++;
            let address = hierarchy + (resolution * resolution - 1u) / 3u + u32(pixel.y) * resolution + u32(pixel.x);
            let node = layerNumeric(vec2u(address % width, address / width));
            if node.a == 0u || !layerContainsZ(time, origin.z, inverseZ, parallelZ, f32(node.r) / 255.0, f32(node.g) / 255.0) { continue; }
            for (var interval = 0u; interval < min(node.b, intervals); interval++) {
                let pair = layerNumeric(layerTile(axis * intervals + interval, vec2u(pixel), pairBase));
                if layerContainsZ(time, origin.z, inverseZ, parallelZ, layerDepth(pair.b, pair.a), layerDepth(pair.r, pair.g)) {
                    return vec4u(vec2u(pixel), 1u, used);
                }
            }
        }
    }
    return vec4u(0u, 0u, 0u, used);
}
fn sampleLayeredSurface(input: VertexOutput) -> Surface {
    var result: Surface;
    if input.weights.w > 0.0 { result.traceFailed = true; return result; }
    let ray = safeNormalize(input.objectPoint - input.objectCamera);
    var axis = 0u;
    if abs(ray.y) > abs(ray.x) { axis = 1u; }
    if abs(ray.z) > abs(ray[axis]) { axis = 2u; }
    let capture = impostor.views[axis];
    let local = (input.objectCamera - impostor.centerRadius.xyz) / impostor.centerRadius.w;
    let origin = vec3f(0.5 + dot(local, capture.right.xyz) * 0.5,
        0.5 - dot(local, capture.up.xyz) * 0.5, 0.5 + dot(local, capture.outward.xyz) * 0.5);
    let direction = vec3f(dot(ray, capture.right.xyz), -dot(ray, capture.up.xyz),
        dot(ray, capture.outward.xyz)) / (2.0 * impostor.centerRadius.w);
    let parallel = abs(direction) < vec3f(1e-8);
    let inverse = vec3f(1.0) / select(direction, vec3f(1.0), parallel);
    let root = layerBox(origin, inverse, parallel, vec3f(0.0), vec3f(1.0));
    if root.y < root.x || root.y <= 0.0 { return result; }
    let resolution = u32(impostor.atlas.w);
    let leafLevel = firstLeadingBit(resolution);
    let intervals = u32(impostor.grid.z);
    let width = u32(impostor.atlas.x);
    let attributeRows = (6u * intervals + 7u) / 8u;
    let pairRows = (3u * intervals + 7u) / 8u;
    let pairBase = attributeRows * u32(impostor.atlas.z);
    let hierarchy = (attributeRows + pairRows) * u32(impostor.atlas.z) * width +
        axis * ((4u * resolution * resolution - 1u) / 3u);
    var cursor = root.x;
    var level = 0u;
    var visits = 0u;
    var owner = layerOwner(origin.xy, direction.xy, inverse.xy, parallel.xy, cursor, resolution, true);
    let backward = layerOwner(origin.xy, direction.xy, inverse.xy, parallel.xy, cursor, resolution, false);
    if cursor > 1e-6 && (any(owner != backward) || root.x == root.y) {
        let contact = layerPoint(origin, inverse.z, parallel.z, cursor, owner, backward, resolution, intervals, axis, hierarchy, pairBase, width, 256u);
        visits += contact.w;
        if contact.z == 1u { return layerAttributes(input.objectCamera + ray * cursor, axis, contact.xy); }
    }
    var leaf = vec2u(clamp(owner, vec2i(0), vec2i(i32(resolution) - 1)));
    for (var iteration = 0u; iteration < 256u; iteration++) {
        if visits >= 256u { result.traceFailed = true; return result; }
        let side = 1u << level;
        let pixel = leaf >> vec2u(leafLevel - level);
        let lower = vec2f(pixel) / f32(side);
        let upper = vec2f(pixel + vec2u(1u)) / f32(side);
        let boundary = select(lower, upper, direction.xy > vec2f(0.0));
        let exits = select((boundary - origin.xy) * inverse.xy, vec2f(1e20), parallel.xy);
        let leave = min(root.y, min(exits.x, exits.y));
        if leave < cursor { result.traceFailed = true; return result; }
        let address = hierarchy + (side * side - 1u) / 3u + pixel.y * side + pixel.x;
        let node = layerNumeric(vec2u(address % width, address / width));
        visits++;
        var bounds = vec2f(cursor, leave);
        if parallel.z {
            if origin.z < f32(node.r) / 255.0 || origin.z > f32(node.g) / 255.0 { bounds.y = -1.0; }
        } else {
            let first = (f32(node.r) / 255.0 - origin.z) * inverse.z;
            let second = (f32(node.g) / 255.0 - origin.z) * inverse.z;
            bounds = vec2f(max(cursor, min(first, second)), min(leave, max(first, second)));
        }
        if node.a != 0u && node.b != 0u && bounds.y >= bounds.x {
            if side != resolution {
                // The node's conservative Z range proves the preceding segment
                // empty. Jump within this XY node before descending. Exact cell
                // contacts retain the original path and its closed-hit rules.
                if bounds.x > cursor && bounds.x < leave {
                    let forward = layerOwner(origin.xy, direction.xy, inverse.xy, parallel.xy, bounds.x, resolution, true);
                    let backward = layerOwner(origin.xy, direction.xy, inverse.xy, parallel.xy, bounds.x, resolution, false);
                    if all(forward == backward) {
                        leaf = vec2u(clamp(forward, vec2i(0), vec2i(i32(resolution) - 1)));
                        cursor = bounds.x;
                    }
                }
                level++;
                continue;
            }
            var nearest = 1e20;
            for (var interval = 0u; interval < min(node.b, intervals); interval++) {
                let pair = layerNumeric(layerTile(axis * intervals + interval, pixel, pairBase));
                let front = layerDepth(pair.r, pair.g);
                let back = layerDepth(pair.b, pair.a);
                var hit = bounds;
                if parallel.z {
                    if origin.z < back || origin.z > front { continue; }
                } else {
                    let first = (back - origin.z) * inverse.z;
                    let second = (front - origin.z) * inverse.z;
                    hit = vec2f(max(bounds.x, min(first, second)), min(bounds.y, max(first, second)));
                }
                let distance = select(hit.x, hit.y, hit.x <= 1e-6);
                if hit.y >= hit.x && distance > 0.0 && distance < nearest { nearest = distance; }
            }
            if nearest < 1e20 { return layerAttributes(input.objectCamera + ray * nearest, axis, leaf); }
        }
        owner = layerOwner(origin.xy, direction.xy, inverse.xy, parallel.xy, leave, resolution, true);
        let before = layerOwner(origin.xy, direction.xy, inverse.xy, parallel.xy, leave, resolution, false);
        if leave > 1e-6 && all(owner != before) {
            let contact = layerPoint(origin, inverse.z, parallel.z, leave, owner, before, resolution, intervals, axis, hierarchy, pairBase, width, 256u - visits);
            visits += contact.w;
            if contact.z == 2u { result.traceFailed = true; return result; }
            if contact.z == 1u { return layerAttributes(input.objectCamera + ray * leave, axis, contact.xy); }
        }
        if leave >= root.y { return result; }
        // Crossed axes use exact integer node edges, avoiding cancellation stalls.
        let span = resolution / side;
        if exits.x == leave { owner.x = select(i32(pixel.x * span) - 1, i32((pixel.x + 1u) * span), direction.x > 0.0); }
        if exits.y == leave { owner.y = select(i32(pixel.y * span) - 1, i32((pixel.y + 1u) * span), direction.y > 0.0); }
        if any(owner < vec2i(0)) || any(owner >= vec2i(i32(resolution))) { return result; }
        let next = vec2u(owner);
        let changed = (leaf.x ^ next.x) | (leaf.y ^ next.y);
        if changed == 0u || leave <= cursor { result.traceFailed = true; return result; }
        level = min(level, leafLevel - firstLeadingBit(changed));
        leaf = next;
        cursor = leave;
    }
    result.traceFailed = true;
    return result;
}
fn sampleSurface(input: VertexOutput) -> Surface {
    if u32(impostor.bounds.w) == 5u { return sampleLayeredSurface(input); }
    if u32(impostor.bounds.w) == 4u { return sampleOctahedralSurface(input); }
    return sampleAngularSurface(input);
}
fn illuminate(base: vec3f, normal: vec3f, worldPosition: vec3f) -> vec3f {
    let view = safeNormalize(material.camera.xyz - worldPosition);
    let roughness = clamp(impostor.settings.z, 0.0, 1.0);
    var ambient = vec3f(0.02);
    var diffuse = vec3f(0.0);
    var specular = vec3f(0.0);
    for (var index = 0u; index < u32(material.camera.w); index++) {
        let light = material.lights[index];
        let direction = safeNormalize(light.direction.xyz);
        if light.direction.w == 1.0 {
            ambient += mix(light.ground.rgb, light.color.rgb, clamp(dot(normal,direction)*0.5+0.5,0.0,1.0)) * light.color.w;
        } else {
            var towardLight = -direction;
            var attenuation = 1.0;
            if light.direction.w == 3.0 {
                let delta = light.direction.xyz - worldPosition;
                towardLight = safeNormalize(delta);
                let falloff = max(1.0 - dot(delta, delta) / (light.ground.x * light.ground.x), 0.0);
                attenuation = falloff * falloff;
            }
            let amount = max(dot(normal,towardLight),0.0);
            diffuse += light.color.rgb * light.color.w * amount * attenuation;
            let halfway = safeNormalize(towardLight+view);
            specular += light.color.rgb * light.color.w * attenuation * pow(max(dot(normal,halfway),0.0),mix(128.0,4.0,roughness)) *
                0.22 * select(0.0,1.0,amount>0.0);
        }
    }
    return max(base*(ambient+diffuse)+specular,vec3f(0.0));
}
struct FragmentOutput {
    @location(0) color: vec4f,
    @builtin(frag_depth) depth: f32,
}
fn fragment(input: VertexOutput, encodeSRGB: bool) -> FragmentOutput {
    let surface = sampleSurface(input);
    if surface.traceFailed { return FragmentOutput(vec4f(1.0, 0.0, 1.0, 1.0), input.position.z); }
    if surface.coverage < impostor.settings.w { discard; }
    let model = mat4x4f(input.model0,input.model1,input.model2,input.model3);
    let world = model * vec4f(surface.point,1.0);
    let normal = safeNormalize(inverseTranspose(model) * surface.normal);
    var color = illuminate(surface.color*material.tint.rgb,normal,world.xyz);
    if encodeSRGB {
        color = select(1.055*pow(color,vec3f(1.0/2.4))-vec3f(0.055),12.92*color,color<=vec3f(0.0031308));
    }
    let clip = material.viewProjection * world;
    var output: FragmentOutput;
    output.color = vec4f(color,1.0);
    output.depth = clamp(clip.z/clip.w,0.0,1.0);
    return output;
}
@fragment fn fs_main(input: VertexOutput) -> FragmentOutput {
    return fragment(input,false);
}
@fragment fn fs_main_srgb(input: VertexOutput) -> FragmentOutput {
    return fragment(input,true);
}
