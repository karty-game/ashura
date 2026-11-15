// Independent implementation of Valve's published RNM basis and irradiance model.
// A deterministic one-bounce ray estimate approximates VRAD's radiosity gathering.
struct Parameters {
    dimensions: vec4u,
    offsets: vec4u,
    extras: vec4u,
    settings: vec4u,
    skyScale: vec4f,
    groundBias: vec4f,
    transportOffsets: vec4u,
    transportCounts: vec4u,
}
struct Point { positionRange: vec4f, energyBias: vec4f }
struct Hit { distance: f32, triangle: u32 }
@group(0) @binding(0) var<uniform> parameters: Parameters;
@group(0) @binding(1) var<storage, read> scene: array<u32>;
@group(0) @binding(2) var<storage, read> points: array<Point>;
@group(0) @binding(3) var<storage, read_write> coefficients: array<vec4f>;
@group(0) @binding(4) var map0: texture_storage_2d<rgba8unorm, write>;
@group(0) @binding(5) var map1: texture_storage_2d<rgba8unorm, write>;
@group(0) @binding(6) var map2: texture_storage_2d<rgba8unorm, write>;
const PI = 3.141592653589793;
const FAR = 1e20;
const NONE = 0xffffffffu;
fn vector(offset: u32) -> vec3f {
    return vec3f(bitcast<f32>(scene[offset]), bitcast<f32>(scene[offset+1u]), bitcast<f32>(scene[offset+2u]));
}
fn triangleA(index: u32) -> vec3f { return vector(parameters.offsets.y + index*12u); }
fn triangleB(index: u32) -> vec3f { return vector(parameters.offsets.y + index*12u+4u); }
fn triangleC(index: u32) -> vec3f { return vector(parameters.offsets.y + index*12u+8u); }
fn box(origin: vec3f, direction: vec3f, distance: f32, offset: u32) -> bool {
    let minimum = vector(offset);
    let maximum = vector(offset+4u);
    var near = 0.0;
    var far = distance;
    for (var axis=0u; axis<3u; axis++) {
        if abs(direction[axis]) < 1e-12 {
            if origin[axis] < minimum[axis] || origin[axis] > maximum[axis] { return false; }
        } else {
            let a = (minimum[axis]-origin[axis])/direction[axis];
            let b = (maximum[axis]-origin[axis])/direction[axis];
            near = max(near, min(a,b));
            far = min(far, max(a,b));
            if near > far { return false; }
        }
    }
    return true;
}
fn intersect(origin: vec3f, direction: vec3f, index: u32) -> f32 {
    let a = triangleA(index);
    let ab = triangleB(index)-a;
    let ac = triangleC(index)-a;
    let p = cross(direction, ac);
    let determinant = dot(ab,p);
    if abs(determinant)<1e-9 { return FAR; }
    let delta = origin-a;
    let u = dot(delta,p)/determinant;
    if u < -1e-6 || u > 1.000001 { return FAR; }
    let q = cross(delta,ab);
    let v = dot(direction,q)/determinant;
    if v < -1e-6 || u+v > 1.000001 { return FAR; }
    let distance = dot(ac,q)/determinant;
    return select(FAR,distance,distance>0.0001);
}
fn trace(origin: vec3f, direction: vec3f, maximum: f32, anyHit: bool) -> Hit {
    var result = Hit(maximum,NONE);
    var stack: array<u32,64>;
    var length=1u;
    stack[0]=0u;
    loop {
        if length==0u { break; }
        length--;
        let offset = parameters.offsets.x+stack[length]*8u;
        if !box(origin,direction,result.distance,offset) { continue; }
        let first=scene[offset+3u];
        let count=scene[offset+7u];
        if count>0u {
            for (var i=0u;i<count;i++) {
                let distance=intersect(origin,direction,first+i);
                if distance<result.distance {
                    result=Hit(distance,first+i);
                    if anyHit { return result; }
                }
            }
        } else {
            stack[length]=first;
            stack[length+1u]=first+1u;
            length+=2u;
        }
    }
    return result;
}
fn environment(direction: vec3f) -> vec3f {
    return mix(parameters.groundBias.xyz,parameters.skyScale.xyz,clamp(direction.y*0.5+0.5,0.0,1.0))/PI;
}
fn hash(value: u32) -> u32 {
    var x=value;
    x=(x^(x>>16u))*0x7feb352du;
    x=(x^(x>>15u))*0x846ca68bu;
    return x^(x>>16u);
}
fn random(value: u32) -> f32 { return f32(hash(value)&0x00ffffffu)/16777216.0; }
fn tangentFor(normal: vec3f) -> vec3f {
    return normalize(cross(select(vec3f(0.0,1.0,0.0),vec3f(1.0,0.0,0.0),abs(normal.y)>.9),normal));
}
fn hemisphere(normal: vec3f, u: f32, v: f32, cosine: bool) -> vec3f {
    let z=select(u,sqrt(u),cosine);
    let radius=sqrt(max(0.0,1.0-z*z));
    let angle=2.0*PI*v;
    let tangent=tangentFor(normal);
    return tangent*(radius*cos(angle))+cross(normal,tangent)*(radius*sin(angle))+normal*z;
}
fn visiblePoint(position: vec3f, normal: vec3f, point: Point) -> vec4f {
    let delta=point.positionRange.xyz-position;
    let squared=dot(delta,delta);
    let range=point.positionRange.w;
    if squared<1e-12 || squared>=range*range { return vec4f(0.0); }
    let distance=sqrt(squared);
    let direction=delta/distance;
    if dot(normal,direction)<=0.0 { return vec4f(0.0); }
    let bias=min(point.energyBias.w,distance*.5);
    if trace(position+direction*bias,direction,distance-bias,true).triangle!=NONE { return vec4f(0.0); }
    let falloff=1.0-squared/(range*range);
    return vec4f(direction,falloff*falloff);
}
fn reflected(position: vec3f, incoming: vec3f, hit: Hit, sampleSeed: u32) -> vec3f {
    let surface=parameters.extras.y+hit.triangle*4u;
    let reflectance=vector(surface);
    var normal=normalize(cross(triangleB(hit.triangle)-triangleA(hit.triangle),triangleC(hit.triangle)-triangleA(hit.triangle)));
    if dot(normal,incoming)>0.0 {
        if bitcast<f32>(scene[surface+3u])<0.5 { return vec3f(0.0); }
        normal=-normal;
    }
    let location=position+incoming*hit.distance;
    var outgoing=vec3f(0.0);
    for (var i=0u;i<parameters.extras.z;i++) {
        let point=points[i];
        let light=visiblePoint(location,normal,point);
        outgoing+=point.energyBias.xyz*(max(dot(normal,light.xyz),0.0)*light.w);
    }
    let secondary=hemisphere(normal,random(sampleSeed),random(sampleSeed+1u),true);
    if trace(location+normal*parameters.groundBias.w,secondary,FAR,true).triangle==NONE {
        // Cosine-weighted secondary sample estimates reflected environment irradiance.
        outgoing+=environment(secondary)*PI;
    }
    return reflectance*outgoing/PI;
}
// Prepared transport stores environment escape and diffuse patch form factors.
// Outgoing patch values are reflected irradiance (radiance multiplied by pi).
// Receiver transfer weights already contain their 1/pi hemisphere integral.
@compute @workgroup_size(64,1,1)
fn bake_rnm_patches(@builtin(global_invocation_id) invocation: vec3u) {
    let index=invocation.x;
    if index>=parameters.transportCounts.y { return; }
    let patchOffset=parameters.transportOffsets.z+index*12u;
    let position=vector(patchOffset);
    let normal=vector(patchOffset+4u);
    let reflectance=vector(patchOffset+8u);
    var irradiance=parameters.skyScale.xyz*bitcast<f32>(scene[patchOffset+3u])+
        parameters.groundBias.xyz*bitcast<f32>(scene[patchOffset+7u]);
    for (var lightIndex=0u;lightIndex<parameters.extras.z;lightIndex++) {
        let point=points[lightIndex];
        // Build preparation orients each patch toward its receiving hemisphere.
        // Preserve that physical side; two-sided materials do not transmit light.
        let light=visiblePoint(position,normal,point);
        irradiance+=point.energyBias.xyz*(max(dot(normal,light.xyz),0.0)*light.w);
    }
    coefficients[parameters.transportCounts.z+index]=vec4f(reflectance*irradiance,1.0);
}
fn preparedLighting(pixel: u32) -> array<vec3f,3> {
    var values: array<vec3f,3>;
    let receiverIndex=scene[parameters.transportOffsets.x+pixel];
    let receiver=parameters.transportOffsets.y+receiverIndex*8u;
    let first=scene[receiver];
    let count=scene[receiver+1u];
    let sky=vector(receiver+2u);
    let ground=vector(receiver+5u);
    for (var basis=0u;basis<3u;basis++) {
        values[basis]=parameters.skyScale.xyz*sky[basis]+parameters.groundBias.xyz*ground[basis];
    }
    if parameters.settings.x>0u {
        for (var link=0u;link<count;link++) {
            let transfer=parameters.transportOffsets.w+(first+link)*4u;
            let patchIndex=scene[transfer];
            let weights=vector(transfer+1u);
            let outgoing=coefficients[parameters.transportCounts.z+patchIndex].xyz;
            for (var basis=0u;basis<3u;basis++) { values[basis]+=outgoing*weights[basis]; }
        }
    }
    return values;
}
@compute @workgroup_size(8,8,1)
fn bake_rnm(@builtin(global_invocation_id) invocation: vec3u) {
    let y=invocation.y+parameters.settings.z;
    if invocation.x>=parameters.dimensions.x || invocation.y>=parameters.settings.w || y>=parameters.dimensions.y { return; }
    let pixel=y*parameters.dimensions.x+invocation.x;
    let receiver=parameters.offsets.z+pixel*8u;
    let coverage=bitcast<f32>(scene[receiver+3u]);
    if coverage<.5 { return; }
    let position=vector(receiver);
    let normal=vector(receiver+4u);
    let frame=parameters.extras.x+pixel*4u;
    let tangent=vector(frame);
    let bitangent=cross(normal,tangent)*bitcast<f32>(scene[frame+3u]);
    var bases: array<vec3f,3>;
    bases[0]=tangent*.816496580927726+normal*.577350269189626;
    bases[1]=tangent*(-.408248290463863)+bitangent*.707106781186548+normal*.577350269189626;
    bases[2]=tangent*(-.408248290463863)-bitangent*.707106781186548+normal*.577350269189626;
    var values: array<vec3f,3>;
    for (var i=0u;i<parameters.extras.z;i++) {
        let point=points[i];
        // Shared visibility for all virtual normals, gated by the physical face.
        let delta=point.positionRange.xyz-position;
        let side=select(1.0,-1.0,coverage>1.5 && dot(normal,delta)<0.0);
        let light=visiblePoint(position,normal*side,point);
        for (var basis=0u;basis<3u;basis++) {
            values[basis]+=point.energyBias.xyz*(max(dot(bases[basis]*side,light.xyz),0.0)*light.w);
        }
    }
    if parameters.transportCounts.x>0u {
        let transported=preparedLighting(pixel);
        for (var basis=0u;basis<3u;basis++) { values[basis]+=transported[basis]; }
    } else {
    let environmentEnabled=any(parameters.skyScale.xyz>vec3f(0.0)) || any(parameters.groundBias.xyz>vec3f(0.0));
    let count=select(0u,parameters.extras.w,environmentEnabled || parameters.settings.x>0u);
    let sequenceSeed=hash(pixel^parameters.settings.y);
    let rotation=random(sequenceSeed);
    for (var sampleIndex=0u;sampleIndex<count;sampleIndex++) {
        let u=(f32(sampleIndex)+.5)/f32(count);
        let v=fract(f32(sampleIndex)*.618033988749895+rotation);
        let direction=hemisphere(normal,u,v,false);
        let origin=position+normal*parameters.groundBias.w;
        let hit=trace(origin,direction,FAR,false);
        var incoming=vec3f(0.0);
        if hit.triangle==NONE { incoming=environment(direction); }
        else if parameters.settings.x>0u { incoming=reflected(origin,direction,hit,sequenceSeed+sampleIndex*2u+1u); }
        for (var basis=0u;basis<3u;basis++) {
            values[basis]+=incoming*(max(dot(bases[basis],direction),0.0)*(2.0*PI/f32(count)));
        }
    }
    }
    for (var basis=0u;basis<3u;basis++) {
        coefficients[pixel*3u+basis]=vec4f(values[basis]/parameters.skyScale.w,1.0);
    }
}
fn encodeTransport(value: vec4f) -> vec4f {
    let peak=max(value.x,max(value.y,value.z));
    if peak<=0.0 { return vec4f(0.0); }
    let exponent=clamp(ceil(log2(peak)),-127.0,127.0);
    return vec4f(value.xyz*exp2(-exponent),(exponent+128.0)/255.0);
}
@compute @workgroup_size(8,8,1)
fn resolve_rnm(@builtin(global_invocation_id) invocation: vec3u) {
    if invocation.x>=parameters.dimensions.x || invocation.y>=parameters.dimensions.y { return; }
    let pixel=invocation.y*parameters.dimensions.x+invocation.x;
    let source=scene[parameters.offsets.w+pixel];
    var a=vec4f(0.0); var b=vec4f(0.0); var c=vec4f(0.0);
    if source!=NONE { a=coefficients[source*3u]; b=coefficients[source*3u+1u]; c=coefficients[source*3u+2u]; }
    if parameters.transportCounts.x>0u { a=encodeTransport(a); b=encodeTransport(b); c=encodeTransport(c); }
    textureStore(map0,vec2i(invocation.xy),a);
    textureStore(map1,vec2i(invocation.xy),b);
    textureStore(map2,vec2i(invocation.xy),c);
}
