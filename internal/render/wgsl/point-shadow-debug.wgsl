@group(0) @binding(0) var shadowFaces: texture_depth_2d_array;
@group(0) @binding(1) var shadowSampler: sampler_comparison;
struct Output { @builtin(position) position: vec4f, @location(0) uv: vec2f, }
@vertex fn vs_main(@builtin(vertex_index) index: u32) -> Output {
 let corners = array<vec2f,3>(vec2f(-1.0,-1.0),vec2f(3.0,-1.0),vec2f(-1.0,3.0));
 let corner=corners[index];
 var output:Output;
 output.position=vec4f(corner*vec2f(0.45,0.30)+vec2f(-0.53,0.66),0.0,1.0);
 output.uv=corner*0.5+0.5;
 return output;
}
fn display(input:Output)->vec3f{
 if any(input.uv<vec2f(0.0)) || any(input.uv>vec2f(1.0)){discard;}
 let tile=vec2f(input.uv.x,1.0-input.uv.y)*vec2f(3.0,2.0);
 let cell=min(vec2u(tile),vec2u(2u,1u));
 let face=cell.y*3u+cell.x;
 let local=fract(tile);
 // Binary depth comparisons expose the actual projected face without
 // requiring a readable depth texture on the compatibility profile.
 var lower=0.0;
 var upper=1.0;
 for(var iteration=0u;iteration<12u;iteration++){
  let middle=(lower+upper)*0.5;
  let visible=textureSampleCompareLevel(shadowFaces,shadowSampler,local,i32(face),middle)>0.5;
  lower=select(lower,middle,visible);
  upper=select(middle,upper,visible);
 }
 let depth=(lower+upper)*0.5;
 let gray=1.0-sqrt(sqrt(max(1.0-depth,0.0)));
 let borders=array<vec3f,6>(vec3f(1.0,0.2,0.2),vec3f(0.6,0.1,0.1),vec3f(0.2,1.0,0.2),vec3f(0.1,0.6,0.1),vec3f(0.2,0.4,1.0),vec3f(0.1,0.2,0.6));
 if any(local<vec2f(0.015)) || any(local>vec2f(0.985)){return borders[face];}
 return vec3f(gray);
}
@fragment fn fs_main(input:Output)->@location(0) vec4f{return vec4f(display(input),1.0);}
@fragment fn fs_main_srgb(input:Output)->@location(0) vec4f{
 let linear=display(input);
 let encoded=select(1.055*pow(linear,vec3f(1.0/2.4))-vec3f(0.055),12.92*linear,linear<=vec3f(0.0031308));
 return vec4f(encoded,1.0);
}
