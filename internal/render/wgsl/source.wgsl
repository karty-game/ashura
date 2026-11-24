struct Light {
    direction: vec4f,
    color: vec4f,
    ground: vec4f,
}
struct SourceUniforms {
    settings: vec4f, // Reserved, SSBump strength, Half-Lambert toggle, RNM decode scale.
    tint: vec4f,
    uvScale: vec4f, // UV0 scale.xy, RNM encoding.z (0: linear, 1: RGBE).
    ambientColors: array<vec4f, 3>,
}
struct FrameUniforms {
    viewProjection: mat4x4f,
    camera: vec4f,
    lights: array<Light, 8>,
}
@group(0) @binding(8) var<uniform> sceneFrame: FrameUniforms;
@group(0) @binding(0) var<uniform> material: SourceUniforms;
@group(0) @binding(1) var surfaceSampler: sampler;
@group(0) @binding(2) var albedoMap: texture_2d<f32>;
@group(0) @binding(3) var ssBumpMap: texture_2d<f32>;
@group(0) @binding(4) var rnmMap0: texture_2d<f32>;
@group(0) @binding(5) var rnmMap1: texture_2d<f32>;
@group(0) @binding(6) var rnmMap2: texture_2d<f32>;
@group(0) @binding(7) var lightmapSampler: sampler;
struct VertexOutput {
    @builtin(position) position: vec4f,
    @location(0) worldPosition: vec3f,
    @location(1) normal: vec3f,
    @location(2) uv: vec2f,
    @location(3) lightmapUV: vec2f,
}
fn safeNormalize(value: vec3f) -> vec3f {
    return value * inverseSqrt(max(dot(value, value), 1e-10));
}
fn inverseTranspose(model: mat4x4f) -> mat3x3f {
    let first = model[0].xyz;
    let second = model[1].xyz;
    let third = model[2].xyz;
    let firstColumn = cross(second, third);
    let secondColumn = cross(third, first);
    let thirdColumn = cross(first, second);
    let determinant = dot(first, firstColumn);
    if abs(determinant) <= 1e-10 {
        return mat3x3f(vec3f(0.0), vec3f(0.0), vec3f(0.0));
    }
    return mat3x3f(firstColumn / determinant, secondColumn / determinant, thirdColumn / determinant);
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
    @location(7) lightmapUV: vec2f,
) -> VertexOutput {
    var output: VertexOutput;
    let model = mat4x4f(model0, model1, model2, model3);
    let world = model * vec4f(position, 1.0);
    output.position = sceneFrame.viewProjection * world;
    output.worldPosition = world.xyz;
    let localNormal = select(normal, vec3f(0.0, 1.0, 0.0), dot(normal, normal) < 1e-10);
    output.normal = inverseTranspose(model) * localNormal;
    output.uv = uv * material.uvScale.xy;
    output.lightmapUV = lightmapUV;
    return output;
}
// Deterministic fallback also used by the offline receiver-frame preparation.
fn fallbackFrame(normal: vec3f) -> mat3x3f {
 let axis = select(vec3f(0.0,1.0,0.0), vec3f(1.0,0.0,0.0),abs(normal.y)>0.9);
 let tangent = normalize(cross(axis,normal));
 return mat3x3f(tangent,cross(normal,tangent),normal);
}
// Cotangent frame from mesh UVs; for tangent-space RNM lighting.
fn tangentFrame(input: VertexOutput) -> mat3x3f {
    let geometric = safeNormalize(input.normal);
    let positionX = dpdx(input.worldPosition);
    let positionY = dpdy(input.worldPosition);
    let uvX = dpdx(input.uv);
    let uvY = dpdy(input.uv);
    let determinant = uvX.x * uvY.y - uvX.y * uvY.x;
    if abs(determinant) < 1e-8 { return fallbackFrame(geometric); }
    let rawTangent = (positionX * uvY.y - positionY * uvX.y) / determinant;
    let rawBitangent = (positionY * uvX.x - positionX * uvY.x) / determinant;
    let projectedTangent = rawTangent - geometric * dot(geometric, rawTangent);
 if dot(projectedTangent,projectedTangent) < 1e-10 { return fallbackFrame(geometric); }
 let tangent = normalize(projectedTangent);
    let handedness = select(-1.0, 1.0, dot(cross(geometric, tangent), rawBitangent) >= 0.0);
    return mat3x3f(tangent, cross(geometric, tangent) * handedness, geometric);
}

// Fixed Source RNM basis; RGB in the SSBump map corresponds to these columns.
const rnmBasis = mat3x3f(
    vec3f(0.81649658, 0.0, 0.57735027),
    vec3f(-0.40824829, 0.70710678, 0.57735027),
    vec3f(-0.40824829, -0.70710678, 0.57735027)
);
// Encoded exponents must never be interpolated. Preserve dim indirect light
// by decoding four level-zero texels and then applying ordinary bilinear weights.
fn decodeRGBE(encoded: vec4f) -> vec3f {
    if encoded.a == 0.0 { return vec3f(0.0); }
    return encoded.rgb * exp2(round(encoded.a * 255.0) - 128.0);
}
fn sampleRGBE(image: texture_2d<f32>, uv: vec2f) -> vec3f {
    let dimensions = vec2i(textureDimensions(image, 0u));
    let position = uv * vec2f(dimensions) - vec2f(0.5);
    let base = vec2i(floor(position));
    let fraction = position - floor(position);
    let minimum = vec2i(0);
    let maximum = dimensions - vec2i(1);
    let lowerLeft = decodeRGBE(textureLoad(image, clamp(base, minimum, maximum), 0));
    let lowerRight = decodeRGBE(textureLoad(image, clamp(base + vec2i(1, 0), minimum, maximum), 0));
    let upperLeft = decodeRGBE(textureLoad(image, clamp(base + vec2i(0, 1), minimum, maximum), 0));
    let upperRight = decodeRGBE(textureLoad(image, clamp(base + vec2i(1, 1), minimum, maximum), 0));
    return mix(mix(lowerLeft, lowerRight, fraction.x), mix(upperLeft, upperRight, fraction.x), fraction.y);
}
fn sourceLighting(input: VertexOutput, frame: mat3x3f, sampled: vec3f, frontFacing: bool) -> vec3f {
    // Reverse all RNM direct-light basis directions on a back face. The authored
    // tangent-space ambient irradiance colors remain per-material values.
    let orientedFrame = frame * select(-1.0, 1.0, frontFacing);
    // Flat unoccluded coefficients sum to one. Never normalize the mapped weights.
    let weights = mix(vec3f(1.0 / 3.0), sampled, clamp(material.settings.y, 0.0, 1.0));
    // Static direct, environment and bounced energy share the same RNM maps.
    // The uniform branch avoids fetching fallback maps in the live-light view.
    var irradiance0 = material.ambientColors[0].rgb;
    var irradiance1 = material.ambientColors[1].rgb;
    var irradiance2 = material.ambientColors[2].rgb;
    if material.settings.w > 0.0 {
        if material.uvScale.z == 1.0 {
            irradiance0 = sampleRGBE(rnmMap0, input.lightmapUV) * material.settings.w;
            irradiance1 = sampleRGBE(rnmMap1, input.lightmapUV) * material.settings.w;
            irradiance2 = sampleRGBE(rnmMap2, input.lightmapUV) * material.settings.w;
        } else {
            irradiance0 = textureSample(rnmMap0, lightmapSampler, input.lightmapUV).rgb * material.settings.w;
            irradiance1 = textureSample(rnmMap1, lightmapSampler, input.lightmapUV).rgb * material.settings.w;
            irradiance2 = textureSample(rnmMap2, lightmapSampler, input.lightmapUV).rgb * material.settings.w;
        }
    }
    var lighting = weights.x * irradiance0 + weights.y * irradiance1 + weights.z * irradiance2;
    let visibility = pointVisibility(input.worldPosition);
    // Source SDK LightmappedGeneric's SSBump flashlight path projects L onto
    // the RNM basis and combines the stored coefficients without normalizing.
    // Reconstructing a unit normal here discards directional visibility.
    for (var index = 0u; index < u32(sceneFrame.camera.w); index++) {
        let light = sceneFrame.lights[index];
        if light.direction.w == 1.0 { continue; }
        var towardLight = -safeNormalize(light.direction.xyz);
        var attenuation = 1.0;
        if light.direction.w == 3.0 {
            let delta = light.direction.xyz - input.worldPosition;
            towardLight = safeNormalize(delta);
            let falloff = max(1.0 - dot(delta, delta) / (light.ground.x * light.ground.x), 0.0);
            attenuation = falloff * falloff;
            if light.ground.y > 0.0 { attenuation *= visibility; }
        }
        let tangentLight = transpose(orientedFrame) * towardLight;
        let projected = transpose(rnmBasis) * tangentLight;
        var diffuse = clamp(dot(weights, projected), 0.0, 1.0);
        if material.settings.z > 0.0 {
            // Optional comparison, not the default LightmappedGeneric path.
            let wrapped = clamp(projected * 0.5 + vec3f(0.5), vec3f(0.0), vec3f(1.0));
            diffuse = dot(weights, wrapped * wrapped);
        }
        lighting += light.color.rgb * light.color.w * attenuation * diffuse;
    }
    return lighting;
}
fn surfaceColor(input: VertexOutput, frontFacing: bool) -> vec3f {
    let base = textureSample(albedoMap, surfaceSampler, input.uv).rgb * material.tint.rgb;
    let shadedBase = base;
    let coefficients = textureSample(ssBumpMap, surfaceSampler, input.uv).rgb;
    return max(shadedBase * sourceLighting(input, tangentFrame(input), coefficients, frontFacing), vec3f(0.0));
}

// Linear output for presentation textures that perform sRGB encoding themselves.
@fragment
fn fs_main(input: VertexOutput, @builtin(front_facing) frontFacing: bool) -> @location(0) vec4f {
    return vec4f(surfaceColor(input, frontFacing), 1.0);
}

// Explicit encoding for unorm presentation textures, including browser canvases.
@fragment
fn fs_main_srgb(input: VertexOutput, @builtin(front_facing) frontFacing: bool) -> @location(0) vec4f {
    let linear = surfaceColor(input, frontFacing);
    let low = 12.92 * linear;
    let high = 1.055 * pow(linear, vec3f(1.0 / 2.4)) - vec3f(0.055);
    let encoded = select(high, low, linear <= vec3f(0.0031308));
    return vec4f(encoded, 1.0);
}
