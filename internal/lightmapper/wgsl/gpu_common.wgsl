// Build-only lightmapper storage ABI @3, packed scene v5. Four storage bindings per stage; trace stages alias readonly scene vectors.
struct LMParams { count:u32, sample:u32, samples:u32, bounces:u32, seed:u32, roulette:u32, exact:u32, emitter:u32, spare0:vec4u, spare1:vec4u };
@group(0) @binding(0) var<uniform> lp:LMParams;
@group(0) @binding(1) var<storage,read> ls:array<u32>;
@group(0) @binding(2) var<storage,read> lt:array<u32>;
struct LMPaths {lmActiveCount:atomic<u32>,lmNextCount:atomic<u32>,lmQueueIndex:u32,lmOverflowFlag:atomic<u32>,data:array<u32>};
@group(0) @binding(3) var<storage,read_write> lpath:LMPaths;
@group(0) @binding(5) var<storage,read_write> lmIndirect:array<u32>;
@group(0) @binding(4) var<storage,read_write> lout:array<u32>;
const LM_PI=3.141592653589793;
const LM_FAR=3.402823466e38;
const LM_NONE=0xffffffffu;
const LM_PATH=128u;
const LM_RESULT=56u;
fn sf(i:u32)->f32{return bitcast<f32>(ls[i]);}
fn sv(i:u32)->vec3f{return vec3f(sf(i),sf(i+1u),sf(i+2u));}
fn tf(i:u32)->f32{return bitcast<f32>(lt[i]);}
fn tv(i:u32)->vec3f{return vec3f(tf(i),tf(i+1u),tf(i+2u));}
fn pf(i:u32)->f32{return bitcast<f32>(lpath.data[pathAddress(i)]);}
fn pv(i:u32)->vec3f{return vec3f(pf(i),pf(i+1u),pf(i+2u));}
fn ps(i:u32,v:f32){lpath.data[pathAddress(i)]=bitcast<u32>(v);}
fn psv(i:u32,v:vec3f){ps(i,v.x);ps(i+1u,v.y);ps(i+2u,v.z);}
fn safeUnit(v:vec3f)->vec3f{let peak=max(abs(v.x),max(abs(v.y),abs(v.z)));if peak==0.0{return vec3f(0.0);}let scaled=v/peak;return scaled/length(scaled);}
fn tangent(n:vec3f)->vec3f{let axis=select(vec3f(0.0,1.0,0.0),vec3f(1.0,0.0,0.0),abs(n.y)>.9);return safeUnit(cross(axis,n));}
fn hash32(v:u32)->u32{var x=v;x=(x^(x>>16u))*0x7feb352du;x=(x^(x>>15u))*0x846ca68bu;return x^(x>>16u);}
const primes=array<u32,64>(2u,3u,5u,7u,11u,13u,17u,19u,23u,29u,31u,37u,41u,43u,47u,53u,59u,61u,67u,71u,73u,79u,83u,89u,97u,101u,103u,107u,109u,113u,127u,131u,137u,139u,149u,151u,157u,163u,167u,173u,179u,181u,191u,193u,197u,199u,211u,223u,227u,229u,233u,239u,241u,251u,257u,263u,269u,271u,277u,281u,283u,293u,307u,311u);
fn randomDimension(path:u32,bounce:u32,dim:u32)->f32{
 let receiver=path%lp.count;
 let rec=lt[1]+receiver*8u;let d=bounce*8u+dim;let base=primes[d%64u];var index=lp.sample+path/lp.count+1u;var factor=1.0/f32(base);var value=0.0;
 loop{if index==0u{break;}value+=f32(index%base)*factor;index/=base;factor/=f32(base);}
 var seed=lp.seed;if lp.spare0.y>0u&&lt[6]!=0u{seed=lt[lt[6]+receiver];}
 let rotate=f32(hash32(lt[rec]^hash32(lt[rec+1u])^seed^(d*0x9e3779b9u))&0xffffffu)/16777216.0;
 return min(fract(value+rotate),0.99999994);
}
fn hemisphere(n:vec3f,u:f32,v:f32,cosine:bool)->vec3f{let z=select(u,sqrt(u),cosine);let r=sqrt(max(0.0,1.0-z*z));let a=2.0*LM_PI*v;let t=tangent(n);return n*z+t*(r*cos(a))+cross(n,t)*(r*sin(a));}
fn powerMIS(a:f32,b:f32)->f32{if a<=0.0{return 0.0;}if b<=0.0{return 1.0;}let m=max(a,b);let x=a/m;let y=b/m;return x*x/(x*x+y*y);}
fn rayBias(p:vec3f)->f32{return 2e-5*max(1.0/sf(19u),max(abs(p.x),max(abs(p.y),abs(p.z))));}
// One reciprocal per ray; expand slab intervals by a conservative float32
// roundoff bound for subtraction, reciprocal and multiplication. Parallel axes
// are admitted separately, so neither infinities nor 0*infinity enter a slab.
fn boxHit(o:vec3f,inverse:vec3f,parallel:vec3<bool>,low:vec3f,high:vec3f,maximum:f32)->bool{
 if any(parallel&((o<low)|(o>high))){return false;}
 let first=(low-o)*inverse;let second=(high-o)*inverse;
 let earliest=min(first,second);let latest=max(first,second);
 let starts=select(earliest-abs(earliest)*4.7683716e-7,vec3f(-LM_FAR),parallel);
 let ends=select(latest+abs(latest)*4.7683716e-7,vec3f(LM_FAR),parallel);
 let near=max(1e-6/sf(19u),max(starts.x,max(starts.y,starts.z)));
 let far=min(maximum,min(ends.x,min(ends.y,ends.z)));
 return near<=far;
}
fn sampleTextureMode(index:u32,uv:vec2f,clampY:bool)->vec3f{
 let rec=ls[10]+(index-1u)*4u;let width=ls[rec];let height=ls[rec+1u];let data=ls[rec+2u];let wrapped=vec2f(fract(uv.x),select(fract(uv.y),clamp(uv.y,0.0,1.0),clampY));let p=wrapped*vec2f(f32(width),f32(height))-.5;let first=vec2i(floor(p));let weight=fract(p);var values:array<vec3f,4>;
 for(var y=0u;y<2u;y++){for(var x=0u;x<2u;x++){let a=((first.x+i32(x))%i32(width)+i32(width))%i32(width);let repeatB=((first.y+i32(y))%i32(height)+i32(height))%i32(height);let b=select(repeatB,clamp(first.y+i32(y),0,i32(height)-1),clampY);values[y*2u+x]=sv(data+(u32(b)*width+u32(a))*3u);}}
 return mix(mix(values[0],values[1],weight.x),mix(values[2],values[3],weight.x),weight.y);
}
fn sampleTexture(index:u32,uv:vec2f)->vec3f{return sampleTextureMode(index,uv,false);}
fn environment(d:vec3f)->vec3f{
 var value=mix(sv(23u),sv(20u),clamp(d.y*.5+.5,0.0,1.0));
 if ls[26]>0u{value=sampleTextureMode(ls[26],vec2f(atan2(d.z,d.x)/(2.0*LM_PI)+.5,atan2(length(d.xz),d.y)/LM_PI),true);}
 if ls[27]>0u{value/=LM_PI;}return value;
}
fn incomingLocal(p:u32,w:vec3f)->vec3f{let n=pv(p+28u);let t=pv(p+32u);let b=cross(n,t)*pf(p+35u);return vec3f(dot(w,t),dot(w,b),dot(w,n));}
fn response(p:u32,w:vec3f)->vec4f{return lmCalibrationMoments(incomingLocal(p,w));}
fn addContribution(p:u32,k:vec4f,value:vec3f,kind:u32){
 if lp.spare0.y>0u{accumulateResponses(p,pv(p+112u),value,2.0*LM_PI);return;}
 for(var i=0u;i<4u;i++){let at=p+72u+i*3u;psv(at,pv(at)+value*k[i]);}
 let at=p+84u+kind*4u;psv(at,pv(at)+value*k.x);
}

override lmPoolABI:u32=1u;
fn poolWidth()->u32{if lmPoolABI!=1u{return 0u;}return max(1u,lp.emitter);}
fn pathCount()->u32{return lp.count*poolWidth();}
fn activeOwner(index:u32)->u32{if index>=atomicLoad(&lpath.lmActiveCount){return LM_NONE;}return lpath.data[pathAddress(pathCount()*pathStride()+lpath.lmQueueIndex*pathCount()+index)];}

fn pathStride()->u32{if lp.spare0.y>0u{return LM_PATH+lp.spare1.x*3u;}return LM_PATH;}
fn resultStride()->u32{if lp.spare0.y>0u{return lp.spare1.x*9u+2u;}return LM_RESULT;}
fn accumulateResponses(p:u32,direction:vec3f,value:vec3f,multiplier:f32){let receiver=p/pathStride();let local=incomingLocal(p,direction);
 for(var i=0u;i<lp.spare1.x;i++){let normal=tv(lt[4]+(receiver*lp.spare1.x+i)*3u);let kernel=max(0.0,dot(normal,local))/LM_PI*multiplier;let at=p+LM_PATH+i*3u;psv(at,pv(at)+value*kernel);}}

// Normalized Welford M2 avoids both E[x²]-E[x]² cancellation and N*variance
// overflow. Return y=0 when float32 cannot represent the requested statistics.
fn lmVarianceUpdate(old:f32,delta:f32,residual:f32,blend:f32)->vec2f{
 let root=sqrt(blend);let a=delta*root;let b=residual*root;
 if !(a==a)||!(b==b)||abs(a)>LM_FAR||abs(b)>LM_FAR{return vec2f(0.0);}
 if abs(a)>1.0&&abs(b)>LM_FAR/abs(a){return vec2f(0.0);}
 let product=a*b;
 if delta!=0.0&&residual!=0.0&&product==0.0{return vec2f(0.0);}
 let retained=old*(1.0-blend);if product>LM_FAR-retained{return vec2f(0.0);}
 return vec2f(retained+product,1.0);
}
fn lmRawSecond(mean:f32,population:f32)->vec2f{
 if abs(mean)>1.8446742e19{return vec2f(0.0);}
 let square=mean*mean;if population>LM_FAR-square{return vec2f(0.0);}
 return vec2f(square+population,1.0);
}

fn safeLength(v:vec3f)->f32{let peak=max(abs(v.x),max(abs(v.y),abs(v.z)));if peak==0.0{return 0.0;}return peak*length(v/peak);}

// Equal fields of adjacent path owners share contiguous storage. Queue entries
// follow the complete state planes and retain their admitted linear addresses.
// This layout affects only build-worker scratch, never scene/tile/result wires.
fn pathAddress(logical:u32)->u32 {
 let stride=pathStride();
 if logical>=pathCount()*stride{return logical;}
 if lp.spare0.y==0u{return (logical&127u)*pathCount()+(logical>>7u);}
 let owner=logical/stride;
 return (logical-owner*stride)*pathCount()+owner;
}

// Diagnostics are disabled by specialization in normal build pipelines. Each
// owner writes its own uint64 pair, so trace calls never contend on an atomic.
override lmProfileRays:bool=false;
fn lmCountRay(p:u32,shadow:bool){
 if !lmProfileRays{return;}
 let lowAt=pathAddress(p+96u);let highAt=pathAddress(p+97u);
 let low=lpath.data[lowAt]+1u;let carry=select(0u,1u,low==0u);let high=lpath.data[highAt]+carry;
 if carry!=0u&&high==0u{atomicStore(&lpath.lmOverflowFlag,1u);}
 lpath.data[lowAt]=low;lpath.data[highAt]=high;
 if shadow{let sl=pathAddress(p+98u);let sh=pathAddress(p+99u);let low=lpath.data[sl]+1u;let carry=select(0u,1u,low==0u);let high=lpath.data[sh]+carry;if carry!=0u&&high==0u{atomicStore(&lpath.lmOverflowFlag,1u);}lpath.data[sl]=low;lpath.data[sh]=high;}
}

@group(0) @binding(6) var<storage,read> scene4:array<vec4u>;
