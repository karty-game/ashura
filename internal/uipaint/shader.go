package uipaint

// Shader is shared by the browser and native UI pipelines.
const Shader = `
struct Record { rect:vec4f, clip:vec4f, color:vec4f, border:vec4f, uv:vec4f, params:vec4f }
@group(0) @binding(0) var<uniform> viewport:vec4f;
@group(0) @binding(1) var<storage,read> records:array<Record>;
@group(1) @binding(0) var page:texture_2d<f32>;
@group(1) @binding(1) var pageSampler:sampler;
struct Output {
 @builtin(position) position:vec4f,
 @location(0) point:vec2f,
 @location(1) uv:vec2f,
 @location(2) @interpolate(flat,either) index:u32
}
@vertex fn vertexMain(@builtin(vertex_index) vertex:u32,@builtin(instance_index) instance:u32,@location(0) rect:vec4f,@location(1) uv:vec4f)->Output {
 let corners=array<vec2f,6>(vec2f(0,0),vec2f(1,0),vec2f(0,1),vec2f(0,1),vec2f(1,0),vec2f(1,1));
 let corner=corners[vertex]; let point=rect.xy+corner*rect.zw;
 return Output(vec4f(point.x/viewport.x*2-1,1-point.y/viewport.y*2,0,1),point,uv.xy+corner*uv.zw,instance);
}
fn distanceToBox(point:vec2f, halfSize:vec2f,radius:f32)->f32 {
 let q=abs(point)-halfSize+radius;
 return length(max(q,vec2f(0)))+min(max(q.x,q.y),0)-radius;
}
fn cross2(a:vec2f,b:vec2f)->f32 { return a.x*b.y-a.y*b.x; }
fn distanceToSegment(point:vec2f,a:vec2f,b:vec2f,radius:f32)->f32 {
 let edge=b-a;
 let fraction=clamp(dot(point-a,edge)/max(dot(edge,edge),0.000001),0.0,1.0);
 return length(point-a-edge*fraction)-radius;
}
fn distanceToTriangle(point:vec2f,a:vec2f,b:vec2f,c:vec2f)->f32 {
 let orientation=select(-1.0,1.0,cross2(b-a,c-a)>=0.0);
 let first=orientation*cross2(b-a,point-a)/max(length(b-a),0.000001);
 let second=orientation*cross2(c-b,point-b)/max(length(c-b),0.000001);
 let third=orientation*cross2(a-c,point-c)/max(length(a-c),0.000001);
 return -min(first,min(second,third));
}
fn distanceToSector(point:vec2f,r:Record)->f32 {
 let local=point-r.border.xy;
 let radial=length(local)-r.params.x;
 if r.params.y>=6.283175 { return radial; }
 let first=cross2(r.uv.xy,local);
 let last=cross2(local,r.uv.zw);
 let angular=select(min(first,last),max(first,last),r.params.y>3.14159265);
 return max(radial,-angular);
}
@fragment fn fragmentMain(v:Output)->@location(0) vec4f {
 let r=records[v.index];
 let halfTexel=vec2f(0.5)/vec2f(textureDimensions(page));
 let imageInset=min(halfTexel,max(r.uv.zw,vec2f(0))*0.5);
 let imageMin=r.uv.xy+imageInset;
 let imageMax=max(imageMin,r.uv.xy+r.uv.zw-imageInset);
 let imageMode=(r.params.z>0.5 && r.params.z<1.5) || (r.params.z>6.5 && r.params.z<7.5);
 let sampleUV=select(v.uv,clamp(v.uv,imageMin,imageMax),imageMode);
 let texel=textureSample(page,pageSampler,sampleUV);
 let screenPxRange=max(0.5*dot(vec2f(r.params.w)/vec2f(textureDimensions(page)),vec2f(1)/max(fwidth(v.uv),vec2f(0.000001))),1.0);
 let center=r.rect.xy+r.rect.zw*0.5;
 let d=distanceToBox(v.point-center,r.rect.zw*0.5,min(r.params.x,min(r.rect.z,r.rect.w)*0.5));
 let aa=max(fwidth(d),0.5); var coverage=1-smoothstep(-aa*0.5,aa*0.5,d);
 let shapeAA=max(max(fwidth(v.point.x),fwidth(v.point.y)),0.0001);
 let inner=1-smoothstep(-aa*0.5,aa*0.5,d+r.params.y);
 var color=mix(r.border,r.color,select(1.0,inner,r.params.y>0));
 if r.params.z>2.5 {
  var distance=0.0;
  if r.params.z<3.5 { distance=distanceToSegment(v.point,r.uv.xy,r.uv.zw,r.params.x); }
  else if r.params.z<4.5 { distance=distanceToTriangle(v.point,r.uv.xy,r.uv.zw,r.params.xy); }
  else if r.params.z<5.5 { distance=distanceToSector(v.point,r); }
  else if r.params.z<6.5 { distance=max(d,length(v.point-r.uv.xy)-r.params.y); }
  else { distance=distanceToBox(v.point-r.border.xy-r.border.zw*0.5,r.border.zw*0.5,min(r.params.x,min(r.border.z,r.border.w)*0.5)); }
  coverage=1-smoothstep(-shapeAA*0.5,shapeAA*0.5,distance);
  color=r.color;
  if imageMode { color*=texel; }
 }
 else if r.params.z>1.5 { let sd=max(min(texel.r,texel.g),min(max(texel.r,texel.g),texel.b)); color.a*=clamp(screenPxRange*(sd-0.5)+0.5,0.0,1.0); }
 else if r.params.z>0 { color*=texel; }
 if any(v.point<r.clip.xy) || any(v.point>=r.clip.xy+r.clip.zw) { discard; }
 color.a*=coverage;
 if viewport.z>0 { color=vec4f(select(color.rgb/12.92,pow((color.rgb+0.055)/1.055,vec3f(2.4)),color.rgb>vec3f(0.04045)),color.a); }
 return vec4f(color.rgb*color.a,color.a);
}`
