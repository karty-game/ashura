struct Shadow { viewProjection: mat4x4f, positionRange: vec4f, }
@group(0) @binding(0) var<uniform> shadow: Shadow;
struct Output {
 @builtin(position) clip: vec4f,
 @location(0) world: vec3f,
}
@vertex fn vs_main(@location(0) position: vec3f,
 @location(3) model0: vec4f, @location(4) model1: vec4f,
 @location(5) model2: vec4f, @location(6) model3: vec4f) -> Output {
 let world=mat4x4f(model0,model1,model2,model3)*vec4f(position,1.0);
 var output:Output;
 output.clip=shadow.viewProjection*world;
 output.world=world.xyz;
 return output;
}
@fragment fn fs_main(input:Output) -> @builtin(frag_depth) f32 {
 return clamp(length(input.world-shadow.positionRange.xyz)/shadow.positionRange.w,0.0,1.0);
}
