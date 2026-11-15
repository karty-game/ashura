package spritepaint

import "github.com/karty-game/karty-webgpu/internal/uipaint"

// Viewport shares the attachment output flag with the UI presentation pipeline.
func Viewport(width, height float32, linear bool) []byte {
	return uipaint.Viewport(width, height, linear)
}

// Shader transforms retained image records, samples nearest prepared sRGB pages,
// tints in linear space and emits premultiplied source-over attachment values.
const Shader = `
struct Record { matrix:vec4f, rect:vec4f, uv:vec4f, color:vec4f, clip:vec4f }
@group(0) @binding(0) var<uniform> viewport:vec4f;
@group(0) @binding(1) var<storage,read> records:array<Record>;
@group(1) @binding(0) var page:texture_2d<f32>;
@group(1) @binding(1) var pageSampler:sampler;
struct Output {
 @builtin(position) position:vec4f,
 @location(0) uv:vec2f,
 @location(1) @interpolate(flat,either) index:u32
}
@vertex fn vertexMain(@builtin(vertex_index) vertex:u32,@builtin(instance_index) instance:u32,
 @location(0) matrix:vec4f,@location(1) rect:vec4f,@location(2) uv:vec4f)->Output {
 let corners=array<vec2f,6>(vec2f(0,0),vec2f(1,0),vec2f(0,1),vec2f(0,1),vec2f(1,0),vec2f(1,1));
 let corner=corners[vertex];let local=corner*rect.zw;
 let point=rect.xy+vec2f(matrix.x*local.x+matrix.z*local.y,matrix.y*local.x+matrix.w*local.y);
 return Output(vec4f(point.x/viewport.x*2-1,1-point.y/viewport.y*2,0,1),uv.xy+corner*uv.zw,instance);
}
fn decodeSRGB(rgb:vec3f)->vec3f {
 return select(rgb/12.92,pow((rgb+0.055)/1.055,vec3f(2.4)),rgb>vec3f(0.04045));
}
fn encodeSRGB(rgb:vec3f)->vec3f {
 return select(rgb*12.92,1.055*pow(max(rgb,vec3f(0)),vec3f(1.0/2.4))-0.055,rgb>vec3f(0.0031308));
}
@fragment fn fragmentMain(v:Output)->@location(0) vec4f {
 let r=records[v.index];
 let halfTexel=vec2f(0.5)/vec2f(textureDimensions(page));
 let lower=r.uv.xy+halfTexel;let upper=max(lower,r.uv.xy+r.uv.zw-halfTexel);
 let sample=textureSample(page,pageSampler,clamp(v.uv,lower,upper));
 var rgb=decodeSRGB(sample.rgb)*r.color.rgb;
 if viewport.z==0 { rgb=encodeSRGB(rgb); }
 let alpha=sample.a*r.color.a;
 return vec4f(rgb*alpha,alpha);
}`
