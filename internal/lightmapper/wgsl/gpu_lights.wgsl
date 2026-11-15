struct LMLight { direction:vec3f,radiance:vec3f,distance:f32,pdf:f32,delta:u32,index:u32 };
fn lightWeight(x:vec3f,index:u32)->f32{
 let at=ls[14]+index*16u;let scale=max(sf(ls[14]+10u),sf(ls[14]+11u));let globalWeight=sf(at+11u)/scale;
 if sf(at+10u)==0.0{return globalWeight;}
 let d=max(max(sv(at)-x,x-sv(at+4u)),vec3f(0.0));let extent=sv(at+4u)-sv(at);let metres=sf(19u);let physicalD=d*metres;let physicalExtent=extent*metres;
 var spatial=(sf(at+10u)/scale)/(dot(physicalD,physicalD)+.25*dot(physicalExtent,physicalExtent)+1e-4);
 if sf(at+15u)>-1.0{let center=sv(at)*.5+sv(at+4u)*.5;let delta=x-center;let dist=safeLength(delta);let radius=.5*safeLength(extent);if !(dist==dist)||!(radius==radius)||dist>LM_FAR||radius>LM_FAR{return globalWeight+spatial;}if dist>radius{let angle=acos(clamp(dot(sv(at+12u),delta/dist),-1.0,1.0));let coneHalf=acos(clamp(sf(at+15u),-1.0,1.0))+asin(clamp(radius/dist,0.0,1.0));spatial*=max(.03125,cos(max(0.0,angle-coneHalf)));}}
 return globalWeight+spatial;
}
fn branch(x:vec3f,index:u32)->f32{let at=ls[14]+index*16u;let l=lightWeight(x,ls[at+3u]);let r=lightWeight(x,ls[at+7u]);if l+r==0.0{return .5;}return clamp(l/(l+r),.03125,.96875);}
fn selectLight(x:vec3f,input:f32)->vec2f{
 if ls[15]==0u{return vec2f(0.0);}var index=0u;var mass=1.0;var u=input;
 loop{let at=ls[14]+index*16u;if ls[at+9u]>0u{return vec2f(f32(ls[ls[16]+ls[at+8u]]),mass);}let left=branch(x,index);if u<left{u/=left;mass*=left;index=ls[at+3u];}else{u=(u-left)/(1.0-left);mass*=1.0-left;index=ls[at+7u];}}
 // Validated finite hierarchies always reach a leaf; retain an explicit return for native WGSL validators.
 return vec2f(0.0);
}
fn lightPMF(x:vec3f,emitter:u32)->f32{
 if lp.exact>0u{return 1.0;}var index=ls[ls[17]+emitter];var mass=1.0;
 loop{if index==0u{break;}let parent=ls[ls[18]+index];let probability=branch(x,parent);let node=ls[14]+parent*16u;mass*=select(1.0-probability,probability,ls[node+3u]==index);index=parent;}return mass;
}
fn cdfSelect(start:u32,count:u32,u:f32)->vec3f{
 var lo=0u;var hi=count-1u;loop{if hi-lo<=1u{break;}let middle=(lo+hi)/2u;if sf(start+middle)>u{hi=middle;}else{lo=middle;}}
 let a=sf(start+lo);let b=sf(start+lo+1u);return vec3f(f32(lo),(u-a)/(b-a),b-a);
}
fn environmentPDF(d:vec3f)->f32{
 if ls[30]==0u{return 1.0/(4.0*LM_PI);}let tex=ls[10]+(ls[26]-1u)*4u;let width=ls[tex];let height=ls[tex+1u];let u=atan2(d.z,d.x)/(2.0*LM_PI)+.5;let v=atan2(length(d.xz),d.y)/LM_PI;let x=min(width-1u,u32(u*f32(width)));let y=min(height-1u,u32(v*f32(height)));let rowMass=sf(ls[29]+y+1u)-sf(ls[29]+y);let column=ls[31]+y*(width+1u);let columnMass=sf(column+x+1u)-sf(column+x);let solid=2.0*LM_PI/f32(width)*(2.0*sin(LM_PI*(f32(y)+.5)/f32(height))*sin(.5*LM_PI/f32(height)));return rowMass*columnMass/solid;
}
fn sampleEnvironment(u:f32,v:f32)->vec4f{
 if ls[30]==0u{let y=1.0-2.0*u;let r=sqrt(max(0.0,1.0-y*y));let a=2.0*LM_PI*v;return vec4f(r*cos(a),y,r*sin(a),1.0/(4.0*LM_PI));}
 let tex=ls[10]+(ls[26]-1u)*4u;let width=ls[tex];let height=ls[tex+1u];let row=cdfSelect(ls[29],height+1u,u);let y=u32(row.x);let column=cdfSelect(ls[31]+y*(width+1u),width+1u,v);let x=u32(column.x);let theta0=LM_PI*f32(y)/f32(height);let theta1=LM_PI*f32(y+1u)/f32(height);let deltaCos=2.0*sin((theta0+theta1)*.5)*sin((theta1-theta0)*.5);var cy=0.0;var r=0.0;if y<height/2u{let oneMinus=2.0*pow(sin(theta0*.5),2.0)+deltaCos*row.y;cy=1.0-oneMinus;r=sqrt(max(0.0,oneMinus*(2.0-oneMinus)));}else{let onePlus=2.0*pow(cos(theta1*.5),2.0)+deltaCos*(1.0-row.y);cy=-1.0+onePlus;r=sqrt(max(0.0,onePlus*(2.0-onePlus)));}let phi=2.0*LM_PI*((f32(x)+column.y)/f32(width)-.5);let solid=2.0*LM_PI/f32(width)*deltaCos;return vec4f(r*cos(phi),cy,r*sin(phi),row.z*column.z/solid);
}
fn emitterArea(index:u32)->f32{
 let at=ls[12]+index*32u;let kind=ls[at];let radius=sf(at+7u);
 if kind==3u{return sf(at+20u)*sf(at+21u);}if kind==4u{return LM_PI*radius*radius;}if kind==5u{return 4.0*LM_PI*radius*radius;}if kind==6u{let tri=ls[6]+ls[at+3u]*32u;return length(cross(sv(tri+4u)-sv(tri),sv(tri+8u)-sv(tri)))*.5;}return 0.0;
}
fn sampleEmitter(x:vec3f,index:u32,u:f32,v:f32)->LMLight{
 let at=ls[12]+index*32u;let kind=ls[at];let position=sv(at+4u);let normal=sv(at+8u);let intensity=sv(at+16u);var result=LMLight(vec3f(0.0),vec3f(0.0),LM_FAR,1.0,1u,index);
 if kind==9u{let env=sampleEnvironment(u,v);result.direction=env.xyz;result.pdf=env.w;result.radiance=environment(env.xyz);result.delta=0u;return result;}
 if kind==7u{result.direction=-normal;result.radiance=intensity;return result;}
 if kind==8u{let axis=-normal;let halfSin=sin(sf(at+7u)*.5);let cap=2.0*halfSin*halfSin;let loss=u*cap;let z=1.0-loss;let r=sqrt(max(0.0,loss*(2.0-loss)));let a=2.0*LM_PI*v;let t=tangent(axis);result.direction=axis*z+t*(r*cos(a))+cross(axis,t)*(r*sin(a));result.pdf=1.0/(2.0*LM_PI*cap);result.radiance=intensity;result.delta=0u;return result;}
 var point=position;var n=normal;let t=sv(at+12u);
 if kind==3u{point+=t*((u-.5)*sf(at+20u))+cross(normal,t)*((v-.5)*sf(at+21u));}
 else if kind==4u{let radius=sf(at+7u)*sqrt(u);let a=2.0*LM_PI*v;point+=t*(radius*cos(a))+cross(normal,t)*(radius*sin(a));}
 else if kind==5u{let y=1.0-2.0*u;let r=sqrt(max(0.0,1.0-y*y));let a=2.0*LM_PI*v;n=vec3f(r*cos(a),y,r*sin(a));point+=n*sf(at+7u);}
 else if kind==6u{let tri=ls[6]+ls[at+3u]*32u;let root=sqrt(u);point=sv(tri)*(1.0-root)+sv(tri+4u)*(root*(1.0-v))+sv(tri+8u)*(root*v);n=safeUnit(cross(sv(tri+4u)-sv(tri),sv(tri+8u)-sv(tri)));}
 let delta=point-x;let distance=length(delta);if distance<=0.0{return result;}result.direction=delta/distance;result.distance=distance;
 let range=sf(at+11u);if range>0.0&&distance>=range{return result;}
 if kind==1u||kind==2u{let physicalDistance=distance*sf(19u);var factor=1.0/(physicalDistance*physicalDistance);if ls[at+1u]==1u{let f=max(0.0,1.0-distance*distance/(range*range));factor=f*f;}var spot=1.0;if kind==2u{let c=dot(normal,-result.direction);let outer=sf(at+19u);let inner=sf(at+15u);spot=select(smoothstep(outer,inner,c),select(0.0,1.0,c>inner),inner==outer);}result.radiance=intensity*(factor*spot);return result;}
 result.delta=0u;var cosine=dot(n,-result.direction);if ls[at+2u]>0u{cosine=abs(cosine);}if cosine<=0.0{result.pdf=0.0;return result;}result.pdf=distance*distance/(emitterArea(index)*cosine);result.radiance=intensity;return result;
}
fn emitterPDF(position:vec3f,index:u32,direction:vec3f,distance:f32,normal:vec3f)->f32{
 let at=ls[12]+index*32u;let kind=ls[at];if kind==9u{return environmentPDF(direction);}if kind==8u{let axis=-sv(at+8u);if atan2(length(cross(axis,direction)),dot(axis,direction))>sf(at+7u){return 0.0;}let halfSin=sin(sf(at+7u)*.5);return 1.0/(4.0*LM_PI*halfSin*halfSin);}if kind==1u||kind==2u||kind==7u{return 0.0;}
 if sf(at+11u)>0.0&&distance>=sf(at+11u){return 0.0;}var cosine=dot(normal,-direction);if ls[at+2u]>0u{cosine=abs(cosine);}if cosine<=0.0{return 0.0;}return distance*distance/(emitterArea(index)*cosine);
}
