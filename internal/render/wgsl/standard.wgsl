struct Light {
    direction: vec4f,
    color: vec4f,
    ground: vec4f,
}
struct StandardUniforms {
    tint: vec4f,
    parameters: vec4f,
    occlusion: vec4f,
    probeOrigin: vec4f,
    probeMinimum: vec4f,
    probeMaximum: vec4f,
}
struct FrameUniforms {
    viewProjection: mat4x4f,
    camera: vec4f,
    lights: array<Light, 8>,
}
@group(0) @binding(8) var<uniform> sceneFrame: FrameUniforms;
@group(0) @binding(0) var<uniform> material: StandardUniforms;
@group(0) @binding(1) var surfaceSampler: sampler;
@group(0) @binding(2) var albedoMap: texture_2d<f32>;
@group(0) @binding(3) var normalMap: texture_2d<f32>;
@group(0) @binding(4) var aoMap: texture_2d<f32>;
@group(0) @binding(5) var reflectionMap: texture_2d<f32>;
@group(0) @binding(6) var lightmapMap: texture_2d<f32>;
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
    output.uv = uv * material.parameters.zw;
    output.lightmapUV = lightmapUV;
    return output;
}
// Cotangent frame from mesh UVs; for tangent-space RGB normals.
fn tangentFrame(input: VertexOutput) -> mat3x3f {
    let geometric = safeNormalize(input.normal);
    let positionX = dpdx(input.worldPosition);
    let positionY = dpdy(input.worldPosition);
    let uvX = dpdx(input.uv);
    let uvY = dpdy(input.uv);
    let determinant = uvX.x * uvY.y - uvX.y * uvY.x;
    if abs(determinant) < 1e-8 { return mat3x3f(vec3f(0.0), vec3f(0.0), geometric); }
    let rawTangent = (positionX * uvY.y - positionY * uvX.y) / determinant;
    let rawBitangent = (positionY * uvX.x - positionX * uvY.x) / determinant;
    let tangent = safeNormalize(rawTangent - geometric * dot(geometric, rawTangent));
    let handedness = select(-1.0, 1.0, dot(cross(geometric, tangent), rawBitangent) >= 0.0);
    return mat3x3f(tangent, cross(geometric, tangent) * handedness, geometric);
}

fn reflectionDirection(point: vec3f, reflectionRay: vec3f, minimum: vec3f, maximum: vec3f) -> vec3f {
    let edge = select(minimum, maximum, reflectionRay >= vec3f(0.0));
    let divisor = select(vec3f(1.0), reflectionRay, abs(reflectionRay) > vec3f(1e-6));
    let distances = select(vec3f(1e20), max((edge - point) / divisor, vec3f(0.0)), abs(reflectionRay) > vec3f(1e-6));
    let distance = min(distances.x, min(distances.y, distances.z));
    return point + reflectionRay * distance - material.probeOrigin.xyz;
}

fn cubemapUV(direction: vec3f) -> vec2f {
    let magnitude = abs(direction);
    var face = 0.0;
    var projected = vec2f(0.0);
    if magnitude.x >= magnitude.y && magnitude.x >= magnitude.z {
        face = select(1.0, 0.0, direction.x >= 0.0);
        projected = vec2f(select(direction.z, -direction.z, direction.x >= 0.0), -direction.y) / max(magnitude.x, 1e-6);
    } else if magnitude.y >= magnitude.z {
        face = select(3.0, 2.0, direction.y >= 0.0);
        projected = vec2f(select(direction.x, -direction.x, direction.y >= 0.0), direction.z) / max(magnitude.y, 1e-6);
    } else {
        face = select(5.0, 4.0, direction.z >= 0.0);
        projected = vec2f(direction.x, select(direction.y, -direction.y, direction.z >= 0.0)) / max(magnitude.z, 1e-6);
    }
    let uv = projected * 0.5 + 0.5;
    return vec2f(face + uv.x, uv.y);
}

fn sampleReflection(point: vec3f, normal: vec3f, view: vec3f, roughness: f32) -> vec3f {
    let strength = clamp(material.occlusion.y, 0.0, 1.0);
    if strength <= 0.0 || any(point < material.probeMinimum.xyz) || any(point > material.probeMaximum.xyz) {
        return vec3f(0.0);
    }
    let ray = reflect(-view, normal);
    let direction = reflectionDirection(point, ray, material.probeMinimum.xyz, material.probeMaximum.xyz);
    let dimensions = textureDimensions(reflectionMap, 0u);
    let atlas = cubemapUV(direction);
    let lod = clamp(roughness * f32(textureNumLevels(reflectionMap) - 1u), 0.0, f32(textureNumLevels(reflectionMap) - 1u));
    let mipSize = vec2f(textureDimensions(reflectionMap, u32(ceil(lod))));
    let inset = 0.5 / mipSize.y;
    let uv = clamp(atlas.y, inset, 1.0 - inset);
    let horizontal = (floor(atlas.x) + clamp(fract(atlas.x), inset, 1.0 - inset)) / 6.0;
    return textureSampleLevel(reflectionMap, surfaceSampler, vec2f(horizontal, uv), lod).rgb;
}

// Calibrated RGB output reuses the existing RGBE numeric representation. Decode
// before interpolation; an exponent is never hardware-filtered as a color.
fn decodeDiffuseRGBE(value: vec4f) -> vec3f {
    if value.a == 0.0 { return vec3f(0.0); }
    return value.rgb * exp2(round(value.a * 255.0) - 128.0);
}
fn sampleDiffuseRGBE(uv: vec2f) -> vec3f {
    let dimensions = vec2i(textureDimensions(lightmapMap, 0u));
    let position = uv * vec2f(dimensions) - vec2f(0.5);
    let base = vec2i(floor(position));
    let fraction = position - floor(position);
    let maximum = dimensions - vec2i(1);
    let a = decodeDiffuseRGBE(textureLoad(lightmapMap, clamp(base, vec2i(0), maximum), 0));
    let b = decodeDiffuseRGBE(textureLoad(lightmapMap, clamp(base + vec2i(1,0), vec2i(0), maximum), 0));
    let c = decodeDiffuseRGBE(textureLoad(lightmapMap, clamp(base + vec2i(0,1), vec2i(0), maximum), 0));
    let d = decodeDiffuseRGBE(textureLoad(lightmapMap, clamp(base + vec2i(1), vec2i(0), maximum), 0));
    return mix(mix(a,b,fraction.x), mix(c,d,fraction.x), fraction.y);
}

fn standardLighting(input: VertexOutput, frame: mat3x3f, sampled: vec3f, base: vec3f, frontFacing: bool) -> vec3f {
    // Use the triangle plane rather than smooth or normal-mapped shading normals
    // for receiver-depth correction, including curved silhouettes and back faces.
    let decoded = sampled * 2.0 - vec3f(1.0);
    // Zero strength disables mapping for every normal map, including tangent
    // normals with zero/negative Z. Retaining that sampled Z could erase or flip
    // the geometric normal in the disabled state.
    let mappedZ = select(decoded.z, 1.0, material.parameters.y == 0.0);
    let mapped = safeNormalize(vec3f(decoded.xy * material.parameters.y, mappedZ));
    // Flip the complete mapped normal, including tangent XY, on back faces.
    // Flipping only the geometric normal before tangent-frame construction
    // would retain the mapped tangent components in the wrong hemisphere.
    let faceSign = select(-1.0, 1.0, frontFacing);
    let normal = safeNormalize(frame * mapped) * faceSign;
    let view = safeNormalize(sceneFrame.camera.xyz - input.worldPosition);
    let roughness = clamp(material.parameters.x, 0.0, 1.0);
    let occlusion = mix(1.0, textureSample(aoMap, surfaceSampler, input.uv).r, clamp(material.occlusion.x, 0.0, 1.0));
    var ambientLight = vec3f(0.02);
    if material.probeOrigin.w >= 2.0 { ambientLight = vec3f(0.0); }
    var diffuseLight = vec3f(0.0);
    var specularLight = vec3f(0.0);
    for (var index = 0u; index < u32(sceneFrame.camera.w); index++) {
        let light = sceneFrame.lights[index];
        let direction = safeNormalize(light.direction.xyz);
        if light.direction.w == 1.0 {
            let blend = clamp(dot(normal, direction) * 0.5 + 0.5, 0.0, 1.0);
            ambientLight += mix(light.ground.rgb, light.color.rgb, blend) * light.color.w;
        } else {
            var towardLight = -direction;
            var attenuation = 1.0;
            if light.direction.w == 3.0 {
                let delta = light.direction.xyz - input.worldPosition;
                let distanceSquared = dot(delta, delta);
                towardLight = safeNormalize(delta);
                let falloff = max(1.0 - distanceSquared / (light.ground.x * light.ground.x), 0.0);
                attenuation = falloff * falloff;
                if light.ground.y > 0.0 { attenuation *= pointVisibility(input.worldPosition); }
            }
            let diffuse = max(dot(normal, towardLight), 0.0);
            let halfway = safeNormalize(towardLight + view);
            let highlight = pow(max(dot(normal, halfway), 0.0), mix(128.0, 4.0, roughness));
            diffuseLight += light.color.rgb * light.color.w * diffuse * attenuation;
            specularLight += light.color.rgb * light.color.w * attenuation * highlight * 0.22 * select(0.0, 1.0, diffuse > 0.0);
        }
    }
    let fresnel = 0.04 + 0.96 * pow(1.0 - clamp(dot(normal, view), 0.0, 1.0), 5.0);
    let reflection = sampleReflection(input.worldPosition, normal, view, roughness);
    var baked = vec3f(0.0);
    if material.occlusion.z > 0.0 {
        if material.probeOrigin.w == 3.0 {
            baked = sampleDiffuseRGBE(input.lightmapUV) * material.occlusion.z;
        } else {
            baked = textureSample(lightmapMap, lightmapSampler, input.lightmapUV).rgb * material.occlusion.z;
        }
    }
    return base * (ambientLight * occlusion + diffuseLight + baked) + specularLight + reflection * fresnel * material.occlusion.y;
}
fn surfaceColor(input: VertexOutput, frontFacing: bool) -> vec4f {
    let albedo = textureSample(albedoMap, surfaceSampler, input.uv);
    if material.occlusion.w > 0.0 {
        let tile = vec2u(floor(input.lightmapUV * 64.0));
        let checker = select(0.25, 0.9, ((tile.x + tile.y) & 1u) == 0u);
        return vec4f(input.lightmapUV, checker, albedo.a);
    }
    let base = albedo.rgb * material.tint.rgb;
    let shadedBase = base;
    let normal = textureSample(normalMap, surfaceSampler, input.uv).rgb;
    let lit = max(standardLighting(input, tangentFrame(input), normal, shadedBase, frontFacing), vec3f(0.0));
    return vec4f(lit, albedo.a);
}

// Evaluate derivative-dependent lighting before discard so all quad lanes retain
// valid texture derivatives. Coverage reuses the existing albedo RGBA sample.
fn alphaMask(surfaceAlpha: f32) {
    if material.probeMaximum.w != 0.0 && surfaceAlpha * material.tint.a < material.probeMinimum.w {
        discard;
    }
}

// Linear output for presentation textures that perform sRGB encoding themselves.
@fragment
fn fs_main(input: VertexOutput, @builtin(front_facing) frontFacing: bool) -> @location(0) vec4f {
    let surface = surfaceColor(input, frontFacing);
    alphaMask(surface.a);
    return vec4f(surface.rgb, 1.0);
}

// Explicit encoding for unorm presentation textures, including browser canvases.
@fragment
fn fs_main_srgb(input: VertexOutput, @builtin(front_facing) frontFacing: bool) -> @location(0) vec4f {
    let surface = surfaceColor(input, frontFacing);
    alphaMask(surface.a);
    let linear = surface.rgb;
    let low = 12.92 * linear;
    let high = 1.055 * pow(linear, vec3f(1.0 / 2.4)) - vec3f(0.055);
    let encoded = select(high, low, linear <= vec3f(0.0031308));
    return vec4f(encoded, 1.0);
}
