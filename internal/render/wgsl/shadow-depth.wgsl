@group(0) @binding(0) var<uniform> viewProjection: mat4x4f;

@vertex fn vs_main(
    @location(0) position: vec3f,
    @location(3) model0: vec4f, @location(4) model1: vec4f,
    @location(5) model2: vec4f, @location(6) model3: vec4f,
) -> @builtin(position) vec4f {
    let world = mat4x4f(model0, model1, model2, model3) * vec4f(position, 1.0);
    return viewProjection * world;
}
