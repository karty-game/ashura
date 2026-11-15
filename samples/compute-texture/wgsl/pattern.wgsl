// Frame data is one explicitly packed, 16-byte uniform vector.
@group(0) @binding(0) var<uniform> frame: vec4f;
@group(0) @binding(1) var output: texture_storage_2d<rgba8unorm, write>;

@compute @workgroup_size(8, 8)
fn generate_pattern(@builtin(global_invocation_id) pixel: vec3u) {
    let size = textureDimensions(output);
    if any(pixel.xy >= size) { return; }

    let uv = (vec2f(pixel.xy) + vec2f(0.5)) / vec2f(size);
    let center = uv - vec2f(0.5);
    let wave = sin(length(center) * 42.0 - frame.x * 2.0);
    let bands = smoothstep(-0.15, 0.15, wave);
    let cool = vec3f(0.025, 0.22, 0.65);
    let warm = vec3f(0.95, 0.32, 0.035);
    let color = mix(cool, warm, bands);
    // Linear RGB: the standard material handles final presentation encoding.
    textureStore(output, vec2i(pixel.xy), vec4f(color, 1.0));
}
