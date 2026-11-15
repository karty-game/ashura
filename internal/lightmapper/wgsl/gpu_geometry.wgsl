fn finalizeHit(value:LMHit)->LMHit{var hit=value;if hit.triangle!=LM_NONE{let source=ls[6]+hit.triangle*32u;hit.emitter=ls[source+7u]-1u;hit.normal=safeUnit(sv(source+28u));}return hit;}
struct LMHit{distance:f32,triangle:u32,emitter:u32,bary:vec3f,normal:vec3f};
fn triangleHit(o:vec3f,d:vec3f,directionLength:f32,index:u32,maximum:f32)->LMHit{
 let at=ls[33]+index*12u;let first=scene4[at>>2u];let second=scene4[(at+4u)>>2u];let third=scene4[(at+8u)>>2u];let triangle=second.w;let a=bitcast<vec3f>(first.xyz);let e1=bitcast<vec3f>(second.xyz);let e2=bitcast<vec3f>(third.xyz);let q=cross(d,e2);let det=dot(e1,q);
 var result=LMHit(maximum,LM_NONE,LM_NONE,vec3f(0.0),vec3f(0.0));if abs(det)<1e-7*bitcast<f32>(first.w)*directionLength{return result;}
 let s=o-a;let u=dot(s,q)/det;if u<0.0||u>1.0{return result;}let r=cross(s,e1);let v=dot(d,r)/det;if v<0.0||u+v>1.0{return result;}
 let dist=dot(e2,r)/det;if dist>=1e-6/sf(19u)&&dist<=maximum{result=LMHit(dist,triangle,LM_NONE,vec3f(1.0-u-v,u,v),vec3f(0.0));}return result;
}
fn analyticHit(o:vec3f,d:vec3f,index:u32,maximum:f32)->LMHit{
 let at=ls[12]+index*32u;let kind=ls[at];let pos=sv(at+4u);let radius=sf(at+7u);let normal=sv(at+8u);
 var dist=LM_FAR;var n=normal;
 if kind==5u{let oc=o-pos;let a=dot(d,d);let b=dot(oc,d);let c=dot(oc,oc)-radius*radius;let disc=b*b-a*c;if disc>=0.0{let root=sqrt(disc);dist=(-b-root)/a;if dist<1e-6/sf(19u){dist=(-b+root)/a;}n=safeUnit(o+d*dist-pos);}}
 else if kind==3u||kind==4u{let denom=dot(d,normal);if abs(denom)>1e-12{dist=dot(pos-o,normal)/denom;let local=o+d*dist-pos;if kind==4u{if dot(local,local)>radius*radius{dist=LM_FAR;}}else{let t=sv(at+12u);let u=dot(local,t);let v=dot(local,cross(normal,t));if abs(u)>sf(at+20u)*.5||abs(v)>sf(at+21u)*.5{dist=LM_FAR;}}}}
 if dist<1e-6/sf(19u)||dist>=maximum{return LMHit(maximum,LM_NONE,LM_NONE,vec3f(0.0),vec3f(0.0));}return LMHit(dist,LM_NONE,index,vec3f(0.0),n);
}
fn surfaceReflectance(triangle:u32,bary:vec3f)->vec3f{
 let at=ls[6]+triangle*32u;let material=ls[8]+ls[at+3u]*16u;var value=sv(material);let tex=ls[material+3u];
 if tex>0u{let uv=vec2f(sf(at+19u),sf(at+23u))*bary.x+vec2f(sf(at+24u),sf(at+25u))*bary.y+vec2f(sf(at+26u),sf(at+27u))*bary.z;value*=sampleTexture(tex,uv);}return value;
}
fn triangleOccludes(o:vec3f,d:vec3f,directionLength:f32,index:u32,maximum:f32)->bool{
 let at=ls[33]+index*12u;let first=scene4[at>>2u];let second=scene4[(at+4u)>>2u];let third=scene4[(at+8u)>>2u];let a=bitcast<vec3f>(first.xyz);let e1=bitcast<vec3f>(second.xyz);let e2=bitcast<vec3f>(third.xyz);let q=cross(d,e2);let det=dot(e1,q);
 if abs(det)<1e-7*bitcast<f32>(first.w)*directionLength{return false;}
 let s=o-a;let u=dot(s,q)/det;if u<0.0||u>1.0{return false;}let r=cross(s,e1);let v=dot(d,r)/det;if v<0.0||u+v>1.0{return false;}
 let dist=dot(e2,r)/det;return dist>=1e-6/sf(19u)&&dist<maximum;
}

// Scene v5: quantized BVH8 nodes and terminal direct triangle records. The
// builder proves depth <=11, including its balanced fallback. One pending group
// per ancestor fits this stack; excess work invalidates the unpublished batch.
override lmSceneABI:u32=5u;
const LM_WIDE_SHARED=8u;
const LM_WIDE_CAPACITY=12u;
var<workgroup> lmWideStack:array<vec2u,512>;
var<private> lmTraceLane:u32;

struct LMWideAxis {near:vec4f,far:vec4f,inside:vec4<bool>};

// Outward-prepared bounds use the affine slab form. This practical image-quality
// profile uses eight f32 epsilons of relative padding and a normal floor,
// accepting rare geometric roundoff instead of proving every intermediate.
// Extreme inputs retain conservative unculling. Packet/stack/queue admission
// is independent; this choice changes only floating-point geometric culling.
fn wideAxis(origin:f32,direction:f32,inverse:f32,p:f32,scale:f32,ql:vec4f,qh:vec4f)->LMWideAxis {
 var result=LMWideAxis(vec4f(-LM_FAR),vec4f(LM_FAR),vec4<bool>(true));
 if !(abs(origin)<=1048576.0)||!(abs(p)<=1048576.0)||!(scale<=1048576.0){return result;}
 let directionBits=bitcast<u32>(direction)&0x7fffffffu;
 if directionBits!=0u&&(directionBits<0x21800000u||directionBits>0x44800000u){return result;}
 if directionBits==0u {
  let low=p+ql*scale;let high=p+qh*scale;
  let positionError=max(max(abs(low),abs(high))*9.536743e-7,vec4f(1.17549435e-38));
  result.inside=(vec4f(origin)>=low-positionError)&(vec4f(origin)<=high+positionError);
  return result;
 }
 let adjusted=scale*inverse;let shifted=(p-origin)*inverse;
 let first=ql*adjusted+shifted;let second=qh*adjusted+shifted;
 let near=min(first,second);let far=max(first,second);
 result.near=near-max(abs(near)*9.536743e-7,vec4f(1.17549435e-38));
 result.far=far+max(abs(far)*9.536743e-7,vec4f(1.17549435e-38));
 return result;
}

fn wideUnpack(value:u32)->vec4f {
 return vec4f(f32(value&255u),f32((value>>8u)&255u),f32((value>>16u)&255u),f32(value>>24u));
}

// Consume each axis before loading the next, limiting live vector values in
// the trace shaders. The low metadata bits address leaves; high bits encode
// internal children in direction-dependent order.
fn wideChildren(at:u32,group:u32,o:vec3f,d:vec3f,inverse:vec3f,maximum:f32,octinv:u32)->u32 {
 let first=scene4[at>>2u];let exponent=first.w;let p=bitcast<vec3f>(first.xyz);
 let scale=vec3f(bitcast<f32>((exponent&255u)<<23u),bitcast<f32>(((exponent>>8u)&255u)<<23u),bitcast<f32>(((exponent>>16u)&255u)<<23u));
 var near=vec4f(1e-6/sf(19u));var far=vec4f(maximum);var valid=vec4<bool>(true);
 {
  let axis=wideAxis(o.x,d.x,inverse.x,p.x,scale.x,wideUnpack(ls[at+8u+group]),wideUnpack(ls[at+14u+group]));
  near=max(near,axis.near);far=min(far,axis.far);valid=valid&axis.inside;
 }
 {
  let axis=wideAxis(o.y,d.y,inverse.y,p.y,scale.y,wideUnpack(ls[at+10u+group]),wideUnpack(ls[at+16u+group]));
  near=max(near,axis.near);far=min(far,axis.far);valid=valid&axis.inside;
 }
 {
  let axis=wideAxis(o.z,d.z,inverse.z,p.z,scale.z,wideUnpack(ls[at+12u+group]),wideUnpack(ls[at+18u+group]));
  near=max(near,axis.near);far=min(far,axis.far);valid=valid&axis.inside;
 }
 valid=valid&(near<=far);
 let metadata=ls[at+6u+group];let inner=(metadata&(metadata<<1u))&0x10101010u;
 let mask=(inner<<4u)-(inner>>4u);
 let index=(metadata^(octinv&mask))&0x1f1f1f1fu;let children=(metadata>>5u)&0x07070707u;
 var hits=0u;
 for(var lane=0u;lane<4u;lane++) {
  if valid[lane]{let shift=lane*8u;hits|=((children>>shift)&7u)<<((index>>shift)&31u);}
 }
 return hits;
}

fn preferWideHit(hit:LMHit,current:LMHit,priority:u32,currentPriority:u32)->bool {
 return hit.triangle!=LM_NONE&&(hit.distance<current.distance||
  (currentPriority!=LM_NONE&&hit.distance==current.distance&&priority<currentPriority));
}

fn wideGeometry(o:vec3f,d:vec3f,maximum:f32,anyHit:bool)->LMHit {
 var result=LMHit(maximum,LM_NONE,LM_NONE,vec3f(0.0),vec3f(0.0));
 if lmSceneABI!=5u||ls[1]!=5u||ls[36]>=LM_WIDE_CAPACITY||ls[37]>1u {
  atomicStore(&lpath.lmOverflowFlag,1u);return result;
 }
 if ls[5]==0u{return result;}
 let directionLength=safeLength(d);var priority=LM_NONE;
 // Up to three primitives need no node. Explicit layout1 also uses this
 // complete AS walk when wide quantization cannot represent admitted geometry
 // or fit the original scene byte bound. It never omits primitive work.
 if ls[3]==0u {
  for(var slot=0u;slot<ls[5];slot++) {
   let hit=triangleHit(o,d,directionLength,slot,result.distance);
   let rank=scene4[(ls[33]+slot*12u+8u)>>2u].w;
   if preferWideHit(hit,result,rank,priority){result=hit;priority=rank;if anyHit{return result;}}
  }
  return result;
 }
 let inverse=vec3f(1.0)/select(d,vec3f(1.0),d==vec3f(0.0));
 let octinv=(7u-(select(0u,4u,d.x<0.0)|select(0u,2u,d.y<0.0)|select(0u,1u,d.z<0.0)))*0x01010101u;
 var nodes=vec2u(0u,0x80000000u);var stack:array<vec2u,4>;
 var stackPtr=0u;
 loop {
  var triangles=vec2u(0u);
  if nodes.y>0x00ffffffu {
   let bits=nodes.y;let childBit=31u-countLeadingZeros(bits);let base=nodes.x;nodes.y&=~(1u<<childBit);
   if nodes.y>0x00ffffffu {
    if stackPtr>=LM_WIDE_CAPACITY{atomicStore(&lpath.lmOverflowFlag,1u);return result;}
    if stackPtr<LM_WIDE_SHARED{lmWideStack[stackPtr*64u+lmTraceLane]=nodes;}else{stack[stackPtr-LM_WIDE_SHARED]=nodes;}
    stackPtr++;
   }
   let slot=(childBit-24u)^(octinv&255u);let relative=countOneBits(bits&~(0xffffffffu<<slot));
   let at=ls[2]+(base+relative)*20u;
   let hits=wideChildren(at,0u,o,d,inverse,result.distance,octinv)|wideChildren(at,1u,o,d,inverse,result.distance,octinv);
   nodes=vec2u(ls[at+4u],(hits&0xff000000u)|(ls[at+3u]>>24u));triangles=vec2u(ls[at+5u],hits&0x00ffffffu);
  }
  loop {
   if triangles.y==0u{break;}let bit=31u-countLeadingZeros(triangles.y);triangles.y&=~(1u<<bit);
   let slot=(triangles.x>>2u)+bit;
   let hit=triangleHit(o,d,directionLength,slot,result.distance);let rank=scene4[(ls[33]+slot*12u+8u)>>2u].w;
   if preferWideHit(hit,result,rank,priority){result=hit;priority=rank;if anyHit{return result;}}
  }
  if nodes.y>0x00ffffffu{continue;}if stackPtr==0u{break;}stackPtr--;
  if stackPtr<LM_WIDE_SHARED{nodes=lmWideStack[stackPtr*64u+lmTraceLane];}else{nodes=stack[stackPtr-LM_WIDE_SHARED];}
 }
 return result;
}

fn wideGeometryOccluded(o:vec3f,d:vec3f,maximum:f32)->bool {
 if lmSceneABI!=5u||ls[1]!=5u||ls[36]>=LM_WIDE_CAPACITY||ls[37]>1u {
  atomicStore(&lpath.lmOverflowFlag,1u);return false;
 }
 if ls[5]==0u{return false;}
 let directionLength=safeLength(d);
 if ls[3]==0u {
  for(var slot=0u;slot<ls[5];slot++){if triangleOccludes(o,d,directionLength,slot,maximum){return true;}}
  return false;
 }
 let inverse=vec3f(1.0)/select(d,vec3f(1.0),d==vec3f(0.0));
 let octinv=(7u-(select(0u,4u,d.x<0.0)|select(0u,2u,d.y<0.0)|select(0u,1u,d.z<0.0)))*0x01010101u;
 var nodes=vec2u(0u,0x80000000u);var stack:array<vec2u,4>;var stackPtr=0u;
 loop {
  var triangles=vec2u(0u);
  if nodes.y>0x00ffffffu {
   let bits=nodes.y;let childBit=31u-countLeadingZeros(bits);let base=nodes.x;nodes.y&=~(1u<<childBit);
   if nodes.y>0x00ffffffu {
    if stackPtr>=LM_WIDE_CAPACITY{atomicStore(&lpath.lmOverflowFlag,1u);return false;}
    if stackPtr<LM_WIDE_SHARED{lmWideStack[stackPtr*64u+lmTraceLane]=nodes;}else{stack[stackPtr-LM_WIDE_SHARED]=nodes;}
    stackPtr++;
   }
   let slot=(childBit-24u)^(octinv&255u);let relative=countOneBits(bits&~(0xffffffffu<<slot));
   let at=ls[2]+(base+relative)*20u;
   let hits=wideChildren(at,0u,o,d,inverse,maximum,octinv)|wideChildren(at,1u,o,d,inverse,maximum,octinv);
   nodes=vec2u(ls[at+4u],(hits&0xff000000u)|(ls[at+3u]>>24u));triangles=vec2u(ls[at+5u],hits&0x00ffffffu);
  }
  loop {
   if triangles.y==0u{break;}let bit=31u-countLeadingZeros(triangles.y);triangles.y&=~(1u<<bit);
   if triangleOccludes(o,d,directionLength,(triangles.x>>2u)+bit,maximum){return true;}
  }
  if nodes.y>0x00ffffffu{continue;}if stackPtr==0u{break;}stackPtr--;
  if stackPtr<LM_WIDE_SHARED{nodes=lmWideStack[stackPtr*64u+lmTraceLane];}else{nodes=stack[stackPtr-LM_WIDE_SHARED];}
 }
 return false;
}

fn traceAnalytic(o:vec3f,d:vec3f,initial:LMHit,anyHit:bool)->LMHit {
 var result=initial;
 if ls[35]==0u{return finalizeHit(result);}
 let parallel=abs(d)<vec3f(1e-20);let inverse=vec3f(1.0)/select(d,vec3f(1.0),parallel);
 var node=select(LM_NONE,0u,ls[15]>0u);var previous=LM_NONE;
 loop {
  if node==LM_NONE{break;}let at=ls[14]+node*16u;let parent=ls[ls[18]+node];var next=parent;
  if previous==parent {
   if boxHit(o,inverse,parallel,sv(at),sv(at+4u),result.distance) {
    if ls[at+9u]==0u{next=ls[at+7u];}
    else{let emitter=ls[ls[16]+ls[at+8u]];let hit=analyticHit(o,d,emitter,result.distance);if hit.emitter!=LM_NONE{result=hit;if anyHit{return finalizeHit(result);}}}
   }
  }else if previous==ls[at+7u]{next=ls[at+3u];}
  previous=node;node=next;
 }
 return finalizeHit(result);
}

fn trace(o:vec3f,d:vec3f,maximum:f32,anyHit:bool)->LMHit {
 let geometry=wideGeometry(o,d,maximum,anyHit);
 if anyHit&&geometry.triangle!=LM_NONE{return finalizeHit(geometry);}
 return traceAnalytic(o,d,geometry,anyHit);
}

fn occluded(o:vec3f,d:vec3f,maximum:f32)->bool {
 if wideGeometryOccluded(o,d,maximum){return true;}
 return traceAnalytic(o,d,LMHit(maximum,LM_NONE,LM_NONE,vec3f(0.0),vec3f(0.0)),true).emitter!=LM_NONE;
}
