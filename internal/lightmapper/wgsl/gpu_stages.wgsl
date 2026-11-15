@compute @workgroup_size(64)
fn lm_generate(@builtin(global_invocation_id) invocation:vec3u){
 let id=invocation.x;if id>=pathCount(){return;}let p=id*pathStride();
 // Only accumulators survive from an earlier sample without a dominating
 // write. Every other live state field is generated or owned by a valid shadow.
 lpath.data[pathAddress(p+7u)]=0u;
 for(var i=72u;i<84u;i++){lpath.data[pathAddress(p+i)]=0u;}
 for(var kind=0u;kind<3u;kind++){for(var axis=0u;axis<3u;axis++){lpath.data[pathAddress(p+84u+kind*4u+axis)]=0u;}}
 if lp.spare0.y>0u{for(var i=LM_PATH;i<pathStride();i++){lpath.data[pathAddress(p+i)]=0u;}}
 if lmProfileRays&&lp.sample==0u{for(var i=96u;i<100u;i++){lpath.data[pathAddress(p+i)]=0u;}}
 lpath.data[pathAddress(pathCount()*pathStride()+id)]=id;
 if lp.sample+id/lp.count>=lp.samples{lpath.data[pathAddress(p+3u)]=0u;return;}
 let rec=lt[1]+(id%lp.count)*8u;let u=randomDimension(id,0u,0u);var lmTarget=u*tf(rec+7u);var lmPatch=lt[rec+4u]+lt[rec+5u]-1u;
 for(var i=0u;i<lt[rec+5u];i++){let candidate=lt[rec+4u]+i;lmTarget-=tf(lt[2]+candidate*40u+3u);if lmTarget<0.0{lmPatch=candidate;break;}}
 let at=lt[2]+lmPatch*40u;let root=sqrt(randomDimension(id,0u,1u));let w=randomDimension(id,0u,2u);let bary=vec3f(1.0-root,root*(1.0-w),root*w);
 let a=tv(at);let b=tv(at+4u);let c=tv(at+8u);let position=a*bary.x+b*bary.y+c*bary.z;var geometric=safeUnit(tv(at+28u));var normal=safeUnit(tv(at+12u)*bary.x+tv(at+16u)*bary.y+tv(at+20u)*bary.z);if dot(normal,normal)==0.0{normal=geometric;}
 let uvA=vec2f(tf(at+15u),tf(at+19u));let uvB=vec2f(tf(at+23u),tf(at+24u));let uvC=vec2f(tf(at+25u),tf(at+26u));let du=uvB-uvA;let dv=uvC-uvA;let det=du.x*dv.y-du.y*dv.x;var rawT=tv(at+32u);var rawB=tv(at+36u);
 if dot(rawT,rawT)==0.0&&abs(det)>1e-20{rawT=((b-a)*dv.y-(c-a)*du.y)/det;rawB=((c-a)*du.x-(b-a)*dv.x)/det;}
 var t=safeUnit(rawT-normal*dot(normal,rawT));if dot(t,t)==0.0{t=tangent(normal);}var hand=select(1.0,-1.0,dot(cross(normal,t),rawB)<0.0);
 if bitcast<i32>(lt[rec+6u])<0{normal=-normal;geometric=-geometric;t=-t;hand=-hand;}
 psv(p,position);lpath.data[pathAddress(p+3u)]=1u;psv(p+8u,vec3f(1.0));psv(p+12u,geometric);psv(p+16u,normal);psv(p+28u,normal);psv(p+32u,t);ps(p+35u,hand);psv(p+36u,geometric);psv(p+40u,position);psv(p+108u,vec3f(1.0));
}

@compute @workgroup_size(64)
fn lm_intersect(@builtin(global_invocation_id) invocation:vec3u){lmTraceLane=invocation.x%64u;
 let id=activeOwner(invocation.x);if id==LM_NONE{return;}let p=id*pathStride();if lpath.data[pathAddress(p+3u)]==0u{return;}let origin=pv(p);let direction=pv(p+4u);let depth=lpath.data[pathAddress(p+7u)];lmCountRay(p,false);let hit=trace(origin,direction,LM_FAR,false);let throughput=pv(p+8u);let kernel=vec4f(pf(p+24u),pf(p+25u),pf(p+26u),pf(p+27u));let previous=pv(p+40u);let previousPDF=pf(p+11u);
 if hit.triangle==LM_NONE&&hit.emitter==LM_NONE{
  if ls[28]>0u{let emitter=ls[28]-1u;let density=environmentPDF(direction)*lightPMF(previous,emitter);let value=throughput*environment(direction)*powerMIS(previousPDF,density);addContribution(p,kernel,value,select(2u,1u,depth==1u));}lpath.data[pathAddress(p+3u)]=0u;return;
 }
 let position=origin+direction*hit.distance;psv(p,position);var geometric=hit.normal;var front=dot(geometric,direction)<0.0;var mayReflect=true;
 if hit.triangle!=LM_NONE{let tri=ls[6]+hit.triangle*32u;let mat=ls[8]+ls[tri+3u]*16u;if !front&&ls[mat+7u]==0u{mayReflect=false;}}
 if hit.emitter!=LM_NONE{let emitter=ls[12]+hit.emitter*32u;if (front||ls[emitter+2u]>0u)&&(sf(emitter+11u)==0.0||hit.distance<sf(emitter+11u)){let density=emitterPDF(previous,hit.emitter,direction,hit.distance,geometric)*lightPMF(previous,hit.emitter);let value=throughput*sv(emitter+16u)*powerMIS(previousPDF,density);addContribution(p,kernel,value,select(2u,0u,depth==1u));}}
 if hit.triangle==LM_NONE||depth>lp.bounces||!mayReflect{lpath.data[pathAddress(p+3u)]=0u;return;}
 let tri=ls[6]+hit.triangle*32u;var normal=safeUnit(sv(tri+12u)*hit.bary.x+sv(tri+16u)*hit.bary.y+sv(tri+20u)*hit.bary.z);psv(p+108u,surfaceReflectance(hit.triangle,hit.bary));
 if !front{geometric=-geometric;normal=-normal;}psv(p+12u,geometric);psv(p+16u,normal);
}

struct LMDirect {ready:bool,direction:vec3f,distance:f32,value:vec3f,kernel:vec4f,category:u32,responseDirection:vec3f,responseMultiplier:f32};
// Both sampled and exhaustive paths share one physical light/MIS evaluation.
fn evaluateLight(id:u32,index:u32,mass:f32)->LMDirect{
 let p=id*pathStride();var result=LMDirect(false,vec3f(0.0),0.0,vec3f(0.0),vec4f(0.0),0u,vec3f(0.0),0.0);
 if lpath.data[pathAddress(p+3u)]==0u||mass<=0.0{return result;}
 let depth=lpath.data[pathAddress(p+7u)];let position=pv(p);let light=sampleEmitter(position,index,randomDimension(id,depth,4u),randomDimension(id,depth,5u));if light.pdf<=0.0||dot(pv(p+12u),light.direction)<=0.0||all(light.radiance<=vec3f(0.0)){return result;}
 let cosine=max(0.0,dot(pv(p+16u),light.direction));if depth>0u&&cosine<=0.0{return result;}let continuationPDF=select(cosine/LM_PI,1.0/(2.0*LM_PI),depth==0u);var weight=1.0;let kind=ls[ls[12]+index*32u];if light.delta==0u&&kind!=8u{weight=powerMIS(mass*light.pdf,continuationPDF);}
 var value=light.radiance*(weight/(mass*light.pdf));var kernel=vec4f(0.0);
 if depth>0u{value*=pv(p+8u)*pv(p+108u)*(cosine/LM_PI);kernel=vec4f(pf(p+24u),pf(p+25u),pf(p+26u),pf(p+27u));}
 else if lp.spare0.y==0u{kernel=response(p,light.direction);}
 return LMDirect(true,light.direction,light.distance*(1.0-2e-5),value,kernel,select(select(0u,1u,kind==9u),2u,depth>0u),select(light.direction,pv(p+112u),depth>0u),select(1.0,2.0*LM_PI,depth>0u));
}
fn prepareLight(id:u32,index:u32,mass:f32){
 let p=id*pathStride();lpath.data[pathAddress(p+55u)]=0u;let direct=evaluateLight(id,index,mass);if !direct.ready{return;}
 psv(p+116u,direct.responseDirection);ps(p+119u,direct.responseMultiplier);psv(p+120u,direct.value);
 let position=pv(p);psv(p+48u,position+pv(p+12u)*rayBias(position));ps(p+51u,direct.distance);psv(p+52u,direct.direction);lpath.data[pathAddress(p+55u)]=1u;
 for(var k=0u;k<4u;k++){psv(p+56u+k*3u,direct.value*direct.kernel[k]);}lpath.data[pathAddress(p+111u)]=direct.category;
}
fn sampledDirect(id:u32){
 let p=id*pathStride();if lpath.data[pathAddress(p+3u)]==0u{return;}
 let selection=selectLight(pv(p),randomDimension(id,lpath.data[pathAddress(p+7u)],3u));let direct=evaluateLight(id,u32(selection.x),selection.y);if !direct.ready{return;}
 let position=pv(p);lmCountRay(p,true);if occluded(position+pv(p+12u)*rayBias(position),direct.direction,direct.distance){return;}
 if lp.spare0.y>0u{accumulateResponses(p,direct.responseDirection,direct.value,direct.responseMultiplier);return;}
 for(var k=0u;k<4u;k++){let at=p+72u+k*3u;psv(at,pv(at)+direct.value*direct.kernel[k]);}
 let category=p+84u+direct.category*4u;psv(category,pv(category)+direct.value*direct.kernel.x);
}
fn consumeShadow(p:u32){if lpath.data[pathAddress(p+55u)]==0u{return;}lpath.data[pathAddress(p+55u)]=0u;lmCountRay(p,true);if occluded(pv(p+48u),pv(p+52u),pf(p+51u)){return;}if lp.spare0.y>0u{accumulateResponses(p,pv(p+116u),pv(p+120u),pf(p+119u));return;}for(var k=0u;k<4u;k++){let at=p+72u+k*3u;psv(at,pv(at)+pv(p+56u+k*3u));}let lmDiagnostic=p+84u+lpath.data[pathAddress(p+111u)]*4u;psv(lmDiagnostic,pv(lmDiagnostic)+pv(p+56u));}

@compute @workgroup_size(64)
fn lm_select(@builtin(global_invocation_id) invocation:vec3u){lmTraceLane=invocation.x%64u;let id=activeOwner(invocation.x);if id==LM_NONE{return;}let p=id*pathStride();if lpath.data[pathAddress(p+3u)]==0u{return;}let selection=selectLight(pv(p),randomDimension(id,lpath.data[pathAddress(p+7u)],3u));prepareLight(id,u32(selection.x),selection.y);}
@compute @workgroup_size(64)
fn lm_shadow(@builtin(global_invocation_id) invocation:vec3u){lmTraceLane=invocation.x%64u;let id=activeOwner(invocation.x);if id==LM_NONE{return;}consumeShadow(id*pathStride());}
// The exhaustive mode is an explicit correctness oracle, never production
// hierarchy selection. Keeping it separate avoids a hidden production scan.
@compute @workgroup_size(64)
fn lm_exact(@builtin(global_invocation_id) invocation:vec3u){lmTraceLane=invocation.x%64u;let id=activeOwner(invocation.x);if id==LM_NONE{return;}for(var i=0u;i<ls[13];i++){prepareLight(id,i,1.0);consumeShadow(id*pathStride());}}

fn continuePath(id:u32){
 let p=id*pathStride();if lpath.data[pathAddress(p+3u)]==0u{return;}let depth=lpath.data[pathAddress(p+7u)];let geometric=pv(p+12u);let normal=pv(p+16u);var direction=vec3f(0.0);var density=1.0/(2.0*LM_PI);var throughput=pv(p+8u);
 if depth>0u{throughput*=pv(p+108u);if all(throughput<=vec3f(0.0)){lpath.data[pathAddress(p+3u)]=0u;return;}if lp.roulette>0u&&depth>=lp.roulette{let survival=clamp(max(throughput.x,max(throughput.y,throughput.z)),.05,.95);if randomDimension(id,depth,8u)>=survival{lpath.data[pathAddress(p+3u)]=0u;return;}throughput/=survival;}direction=hemisphere(normal,randomDimension(id,depth,6u),randomDimension(id,depth,7u),true);if dot(direction,geometric)<=0.0{lpath.data[pathAddress(p+3u)]=0u;return;}density=max(0.0,dot(normal,direction))/LM_PI;}
 else{direction=hemisphere(geometric,randomDimension(id,depth,6u),randomDimension(id,depth,7u),false);psv(p+112u,direction);let kernel=response(p,direction)/density;for(var k=0u;k<4u;k++){ps(p+24u+k,kernel[k]);}}
 let position=pv(p);psv(p+40u,position);psv(p,position+geometric*rayBias(position));psv(p+4u,direction);psv(p+8u,throughput);ps(p+11u,density);lpath.data[pathAddress(p+7u)]=depth+1u;
}

@compute @workgroup_size(64)
fn lm_continue(@builtin(global_invocation_id) invocation:vec3u){let id=activeOwner(invocation.x);if id==LM_NONE{return;}continuePath(id);}
// A sampled owner has no cross-owner dependency between visibility and its
// continuation. Keep that preparation local until compaction is required.
@compute @workgroup_size(64)
fn lm_advance(@builtin(global_invocation_id) invocation:vec3u){lmTraceLane=invocation.x%64u;let id=activeOwner(invocation.x);if id==LM_NONE{return;}sampledDirect(id);continuePath(id);}

@compute @workgroup_size(64)
fn lm_reduce(@builtin(global_invocation_id) invocation:vec3u){
 let id=invocation.x;if id>=lp.count{return;}for(var lane=0u;lane<poolWidth();lane++){if lp.sample+lane>=lp.samples{break;}let p=(lane*lp.count+id)*pathStride();let at=id*resultStride();if lp.spare0.y>0u{let countOffset=at+lp.spare1.x*9u;if atomicLoad(&lpath.lmOverflowFlag)!=0u{lout[countOffset]=0u;return;}let count=lout[countOffset]+1u;let blend=1.0/f32(count);for(var k=0u;k<lp.spare1.x*3u;k++){let value=pf(p+LM_PATH+k);let mean=bitcast<f32>(lout[at+k]);let delta=value-mean;let updated=mean+delta*blend;lout[at+k]=bitcast<u32>(updated);let old=bitcast<f32>(lout[at+lp.spare1.x*3u+k]);let variance=lmVarianceUpdate(old,delta,value-updated,blend);if variance.y==0.0{lout[countOffset+1u]=4u;}lout[at+lp.spare1.x*3u+k]=bitcast<u32>(variance.x);}lout[countOffset]=count;return;}if atomicLoad(&lpath.lmOverflowFlag)!=0u{lout[(33u)*lp.count+id]=0u;return;}let count=lout[(33u)*lp.count+id]+1u;let blend=1.0/f32(count);
 for(var k=0u;k<12u;k++){let value=pf(p+72u+k);let mean=bitcast<f32>(lout[(k)*lp.count+id]);let delta=value-mean;let updated=mean+delta*blend;lout[(k)*lp.count+id]=bitcast<u32>(updated);let moment=bitcast<f32>(lout[(12u+k)*lp.count+id]);let variance=lmVarianceUpdate(moment,delta,value-updated,blend);if variance.y==0.0{lout[(55u)*lp.count+id]=4u;}lout[(12u+k)*lp.count+id]=bitcast<u32>(variance.x);}
 for(var kind=0u;kind<3u;kind++){for(var axis=0u;axis<3u;axis++){let i=(24u+kind*3u+axis)*lp.count+id;let value=pf(p+84u+kind*4u+axis);let previous=bitcast<f32>(lout[i]);lout[i]=bitcast<u32>(previous+(value-previous)*blend);}}lout[(33u)*lp.count+id]=count;
 }
}
@compute @workgroup_size(64)
fn lm_resolve(@builtin(global_invocation_id) invocation:vec3u){
 let id=invocation.x;if id>=lp.count{return;}let at=id*resultStride();
 if lp.spare0.y>0u{let components=lp.spare1.x*3u;let countOffset=at+components*3u;let count=lout[countOffset];if count==0u{return;}if (lout[countOffset+1u]&4u)!=0u{lout[countOffset]=0u;return;}var rawValid=true;
  let factor=f32(count)/f32(max(1u,count-1u));
  for(var k=0u;k<components;k++){let mean=bitcast<f32>(lout[at+k]);let pop=max(0.0,bitcast<f32>(lout[at+components+k]));if pop>LM_FAR/factor{lout[countOffset]=0u;return;}lout[at+components*2u+k]=bitcast<u32>(pop*factor);let raw=lmRawSecond(mean,pop);rawValid=rawValid&&raw.y>0.0;lout[at+components+k]=bitcast<u32>(raw.x);}
  if !rawValid{for(var k=0u;k<components;k++){lout[at+components+k]=0u;}}
  lout[countOffset+1u]=select(0u,1u,count>1u)|select(0u,2u,rawValid);return;
 }
 let count=lout[(33u)*lp.count+id];if count==0u{return;}if (lout[(55u)*lp.count+id]&4u)!=0u{lout[(33u)*lp.count+id]=0u;return;}var rawValid=true;let factor=f32(count)/f32(max(1u,count-1u));
 for(var k=0u;k<12u;k++){let mean=bitcast<f32>(lout[(k)*lp.count+id]);let pop=max(0.0,bitcast<f32>(lout[(12u+k)*lp.count+id]));if pop>LM_FAR/factor{lout[(33u)*lp.count+id]=0u;return;}lout[(43u+k)*lp.count+id]=bitcast<u32>(pop*factor);let raw=lmRawSecond(mean,pop);rawValid=rawValid&&raw.y>0.0;lout[(12u+k)*lp.count+id]=bitcast<u32>(raw.x);}
 if !rawValid{for(var k=0u;k<12u;k++){lout[(12u+k)*lp.count+id]=0u;}}
 lout[(55u)*lp.count+id]=select(0u,1u,count>1u)|select(0u,2u,rawValid);
 if (lp.spare0.x&2u)==0u{return;}
 for(var channel=0u;channel<3u;channel++){let flat=bitcast<f32>(lout[(channel)*lp.count+id]);let rhs=vec3f(bitcast<f32>(lout[(3u+channel)*lp.count+id]),bitcast<f32>(lout[(6u+channel)*lp.count+id]),bitcast<f32>(lout[(9u+channel)*lp.count+id]));let coefficients=lmProjectRNM(flat,rhs);for(var k=0u;k<3u;k++){lout[(34u+k*3u+channel)*lp.count+id]=bitcast<u32>(coefficients[k]);}}
}

// Integer-only compaction preserves owned contribution slots. Queue ordering
// never enters sample identity or floating-point reduction order.
@compute @workgroup_size(64)
fn lm_compact(@builtin(global_invocation_id) invocation:vec3u){
 let id=activeOwner(invocation.x);if id==LM_NONE{return;}if lpath.data[pathAddress(id*pathStride()+3u)]==0u{return;}
 let slot=atomicAdd(&lpath.lmNextCount,1u);if slot>=pathCount(){atomicStore(&lpath.lmOverflowFlag,1u);return;}
 let other=1u-lpath.lmQueueIndex;lpath.data[pathAddress(pathCount()*pathStride()+other*pathCount()+slot)]=id;
}
@compute @workgroup_size(1)
fn lm_reset(){atomicStore(&lpath.lmActiveCount,pathCount());atomicStore(&lpath.lmNextCount,0u);atomicStore(&lpath.lmOverflowFlag,0u);lpath.lmQueueIndex=0u;lmIndirect[0]=(pathCount()+63u)/64u;lmIndirect[1]=1u;lmIndirect[2]=1u;}
@compute @workgroup_size(1)
fn lm_dispatch(){let count=atomicLoad(&lpath.lmNextCount);if count>pathCount(){atomicStore(&lpath.lmOverflowFlag,1u);}
 atomicStore(&lpath.lmActiveCount,min(count,pathCount()));atomicStore(&lpath.lmNextCount,0u);lpath.lmQueueIndex=1u-lpath.lmQueueIndex;
 lmIndirect[0]=(min(count,pathCount())+63u)/64u;lmIndirect[1]=1u;lmIndirect[2]=1u;}

// After resolve no transport state remains live. Serialize the internal SoA
// statistics into dead physical path planes 0..55; preserve profile planes 96..99.
// Verification/reconstruction remain on their existing variable/AoS wires.
@compute @workgroup_size(64)
fn lm_serialize(@builtin(global_invocation_id) invocation:vec3u){
 let id=invocation.x;if id>=lp.count||lp.spare0.y>0u{return;}

 if lmProfileRays{
  var rays=vec2u(0u);var shadows=vec2u(0u);
  for(var lane=0u;lane<poolWidth();lane++){
   let p=(lane*lp.count+id)*pathStride();let low=lpath.data[pathAddress(p+96u)];let high=lpath.data[pathAddress(p+97u)];let sum=rays.x+low;let carry=select(0u,1u,sum<rays.x);let newHigh=rays.y+high;let finalHigh=newHigh+carry;if newHigh<rays.y||finalHigh<newHigh{atomicStore(&lpath.lmOverflowFlag,1u);}rays=vec2u(sum,finalHigh);
   let sl=lpath.data[pathAddress(p+98u)];let sh=lpath.data[pathAddress(p+99u)];let ss=shadows.x+sl;let sc=select(0u,1u,ss<shadows.x);let nh=shadows.y+sh;let fh=nh+sc;if nh<shadows.y||fh<nh{atomicStore(&lpath.lmOverflowFlag,1u);}shadows=vec2u(ss,fh);
  }
  let p=id*pathStride();lpath.data[pathAddress(p+96u)]=rays.x;lpath.data[pathAddress(p+97u)]=rays.y;lpath.data[pathAddress(p+98u)]=shadows.x;lpath.data[pathAddress(p+99u)]=shadows.y;
 }
 for(var k=0u;k<56u;k++){lpath.data[id*56u+k]=lout[k*lp.count+id];}
}
