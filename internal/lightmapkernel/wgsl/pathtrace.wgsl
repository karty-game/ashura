// Offline RNM path tracer. The fourth storage buffer retains exact geometric
// normals, independent of the receiver's interpolated RNM shading frame.
@group(0) @binding(7) var<storage, read> geometricNormals: array<vec4f>;

// Reciprocal direction is computed once per ray. Parallel slabs are handled
// explicitly so origins on a box face never produce 0*infinity NaNs.
fn productionBox(node: u32, origin: vec3f, reciprocal: vec3f, parallel: vec3<bool>, maximumDistance: f32) -> f32 {
    let offset=parameters.offsets.x+node*8u;
    let minimum=vector(offset);
    let maximum=vector(offset+4u);
    if (parallel.x && (origin.x<minimum.x || origin.x>maximum.x)) ||
       (parallel.y && (origin.y<minimum.y || origin.y>maximum.y)) ||
       (parallel.z && (origin.z<minimum.z || origin.z>maximum.z)) { return FAR; }
    let first=(minimum-origin)*reciprocal;
    let second=(maximum-origin)*reciprocal;
    let lower=select(min(first,second),vec3f(-FAR),parallel);
    let upper=select(max(first,second),vec3f(FAR),parallel);
    let near=max(0.0,max(lower.x,max(lower.y,lower.z)));
    let far=min(maximumDistance,min(upper.x,min(upper.y,upper.z)));
    return select(FAR,near,near<=far);
}

// Near-first traversal finds a tight closest-hit bound early. Deferred nodes
// retain their entry distance so a closer hit can prune them without another
// box test. The admitted scene tree has depth below63, fitting this stack.
fn traceProduction(origin: vec3f, direction: vec3f, maximum: f32, anyHit: bool) -> Hit {
    let parallel=abs(direction)<vec3f(1e-12);
    let reciprocal=vec3f(1.0)/select(direction,vec3f(1.0),parallel);
    var result=Hit(maximum,NONE);
    var stack: array<vec2u,64>;
    var length=1u;
    stack[0]=vec2u(0u,bitcast<u32>(productionBox(0u,origin,reciprocal,parallel,maximum)));
    loop {
        if length==0u { break; }
        length--;
        let pending=stack[length];
        let near=bitcast<f32>(pending.y);
        if near==FAR || near>result.distance { continue; }
        let offset=parameters.offsets.x+pending.x*8u;
        let first=scene[offset+3u];
        let count=scene[offset+7u];
        if count>0u {
            for (var index=0u;index<count;index++) {
                let distance=intersect(origin,direction,first+index);
                if distance<result.distance {
                    result=Hit(distance,first+index);
                    if anyHit { return result; }
                }
            }
        } else {
            let left=productionBox(first,origin,reciprocal,parallel,result.distance);
            let right=productionBox(first+1u,origin,reciprocal,parallel,result.distance);
            // The old traversal visits the right sibling first on equal
            // distances. Retain that tie order for coincident primitives.
            let rightFirst=right<=left;
            let nearChild=select(first,first+1u,rightFirst);
            let farChild=select(first+1u,first,rightFirst);
            let nearDistance=min(left,right);
            let farDistance=max(left,right);
            if farDistance!=FAR {
                stack[length]=vec2u(farChild,bitcast<u32>(farDistance));
                length++;
            }
            if nearDistance!=FAR {
                stack[length]=vec2u(nearChild,bitcast<u32>(nearDistance));
                length++;
            }
        }
    }
    return result;
}

fn visiblePointProduction(position: vec3f, normal: vec3f, point: Point) -> vec4f {
    let delta=point.positionRange.xyz-position;
    let squared=dot(delta,delta);
    let range=point.positionRange.w;
    if squared<1e-12 || squared>=range*range { return vec4f(0.0); }
    let distance=sqrt(squared);
    let direction=delta/distance;
    if dot(normal,direction)<=0.0 { return vec4f(0.0); }
    let bias=min(point.energyBias.w,distance*.5);
    if traceProduction(position+direction*bias,direction,distance-bias,true).triangle!=NONE { return vec4f(0.0); }
    let falloff=1.0-squared/(range*range);
    return vec4f(direction,falloff*falloff);
}

// Sum radiance from point lights at every diffuse hit and environment escape.
// Cosine sampling cancels the Lambertian cosine/pdf, leaving reflectance as
// throughput. Bounces counts reflecting surfaces, excluding the receiver.
fn pathRadiance(initialOrigin: vec3f, initialDirection: vec3f, seed: u32) -> vec3f {
    var origin=initialOrigin;
    var direction=initialDirection;
    var throughput=vec3f(1.0);
    var radiance=vec3f(0.0);
    for (var depth=0u;depth<=parameters.settings.x;depth++) {
        let hit=traceProduction(origin,direction,FAR,false);
        if hit.triangle==NONE {
            radiance+=throughput*environment(direction);
            break;
        }
        if depth==parameters.settings.x { break; }
        let surface=parameters.extras.y+hit.triangle*4u;
        let reflectance=vector(surface);
        var normal=normalize(cross(triangleB(hit.triangle)-triangleA(hit.triangle),triangleC(hit.triangle)-triangleA(hit.triangle)));
        if dot(normal,direction)>0.0 {
            if bitcast<f32>(scene[surface+3u])<0.5 { break; }
            normal=-normal;
        }
        let position=origin+direction*hit.distance;
        var irradiance=vec3f(0.0);
        for (var lightIndex=0u;lightIndex<parameters.extras.z;lightIndex++) {
            let point=points[lightIndex];
            let light=visiblePointProduction(position,normal,point);
            irradiance+=point.energyBias.xyz*(max(dot(normal,light.xyz),0.0)*light.w);
        }
        throughput*=reflectance;
        radiance+=throughput*irradiance/PI;
        if all(throughput==vec3f(0.0)) { break; }
        direction=hemisphere(normal,random(seed+depth*2u),random(seed+depth*2u+1u),true);
        origin=position+normal*parameters.groundBias.w;
    }
    return radiance;
}

@compute @workgroup_size(8,8,1)
fn pathtrace_rnm(@builtin(global_invocation_id) invocation: vec3u) {
    let y=invocation.y+parameters.settings.z;
    if invocation.x>=parameters.dimensions.x || invocation.y>=parameters.settings.w || y>=parameters.dimensions.y { return; }
    let pixel=y*parameters.dimensions.x+invocation.x;
    let receiver=parameters.offsets.z+pixel*8u;
    let coverage=bitcast<f32>(scene[receiver+3u]);
    if coverage<.5 { return; }
    let position=vector(receiver);
    let normal=vector(receiver+4u);
    let geometric=geometricNormals[pixel].xyz;
    let frame=parameters.extras.x+pixel*4u;
    let tangent=vector(frame);
    let bitangent=cross(normal,tangent)*bitcast<f32>(scene[frame+3u]);
    var bases: array<vec3f,3>;
    bases[0]=tangent*.816496580927726+normal*.577350269189626;
    bases[1]=tangent*(-.408248290463863)+bitangent*.707106781186548+normal*.577350269189626;
    bases[2]=tangent*(-.408248290463863)-bitangent*.707106781186548+normal*.577350269189626;
    var values: array<vec3f,3>;
    let firstSample=parameters.transportCounts.z;
    if firstSample==0u {
    for (var lightIndex=0u;lightIndex<parameters.extras.z;lightIndex++) {
        let point=points[lightIndex];
        let side=select(1.0,-1.0,coverage>1.5 && dot(geometric,point.positionRange.xyz-position)<0.0);
        let light=visiblePointProduction(position,geometric*side,point);
        for (var basis=0u;basis<3u;basis++) {
            values[basis]+=point.energyBias.xyz*(max(dot(bases[basis]*side,light.xyz),0.0)*light.w);
        }
    }
    } else {
        // Offline dispatches partition the primary sequence. Float32 scratch
        // preserves accumulated coefficients; point lights are added once.
        for (var basis=0u;basis<3u;basis++) {
            values[basis]=coefficients[pixel*3u+basis].xyz*parameters.skyScale.w;
        }
    }
    let count=parameters.extras.w;
    let sequenceSeed=hash(pixel^parameters.settings.y);
    let rotation=random(sequenceSeed);
    let origin=position+geometric*parameters.groundBias.w;
    // Stratify the receiver hemisphere uniformly: RNM virtual normals may
    // extend beyond the geometric normal, so cosine sampling can have high
    // variance near the physical horizon. Weight is 2*pi/N.
    let endSample=min(count,firstSample+parameters.transportCounts.w);
    for (var sampleIndex=firstSample;sampleIndex<endSample;sampleIndex++) {
        let u=(f32(sampleIndex)+.5)/f32(count);
        let v=fract(f32(sampleIndex)*.618033988749895+rotation);
        let direction=hemisphere(geometric,u,v,false);
        let incoming=pathRadiance(origin,direction,hash(sequenceSeed^hash(sampleIndex+1u)));
        for (var basis=0u;basis<3u;basis++) {
            values[basis]+=incoming*(max(dot(bases[basis],direction),0.0)*(2.0*PI/f32(count)));
        }
    }
    for (var basis=0u;basis<3u;basis++) {
        coefficients[pixel*3u+basis]=vec4f(values[basis]/parameters.skyScale.w,1.0);
    }
}

@compute @workgroup_size(8,8,1)
fn resolve_pathtrace_rnm(@builtin(global_invocation_id) invocation: vec3u) {
    if invocation.x>=parameters.dimensions.x || invocation.y>=parameters.dimensions.y { return; }
    let pixel=invocation.y*parameters.dimensions.x+invocation.x;
    let source=scene[parameters.offsets.w+pixel];
    var a=vec4f(0.0); var b=vec4f(0.0); var c=vec4f(0.0);
    if source!=NONE {
        a=encodeTransport(coefficients[source*3u]);
        b=encodeTransport(coefficients[source*3u+1u]);
        c=encodeTransport(coefficients[source*3u+2u]);
    }
    textureStore(map0,vec2i(invocation.xy),a);
    textureStore(map1,vec2i(invocation.xy),b);
    textureStore(map2,vec2i(invocation.xy),c);
}
