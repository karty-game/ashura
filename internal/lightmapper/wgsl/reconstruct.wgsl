// Immutable halo reconstruction ABI @1. Input is lt; only requested interiors
// are written to lout. Explicit edges have already passed surface visibility.
const LM_RECON_RECORD=52u;
fn lmReconRecord(index:u32)->u32{return lt[4]+index*LM_RECON_RECORD;}
fn lmReconWeight(a:u32,b:u32)->f32{
 if lt[a+2u]!=lt[b+2u]||lt[a+3u]!=lt[b+3u]||lt[a+7u]!=lt[b+7u]{return 0.0;}
 let nc=dot(tv(a+8u),tv(b+8u));let tc=dot(tv(a+12u),tv(b+12u));
 if nc<tf(10u)||tc<tf(11u){return 0.0;}
 let delta=tv(b+4u)-tv(a+4u);let distance=dot(delta,delta);let radius=tf(8u);
 if distance>radius*radius||abs(dot(delta,tv(a+8u)))>tf(9u){return 0.0;}
 let difference=tv(a+16u)-tv(b+16u);
 let noise=tv(a+28u)/f32(lt[a+11u])+tv(b+28u)/f32(lt[b+11u]);
 var contrast=0.0;
 for(var c=0u;c<3u;c++){
  if noise[c]<=0.0{if difference[c]!=0.0{return 0.0;}}
  else{contrast+=difference[c]*difference[c]/(tf(12u)*noise[c]);}
 }
 return exp(-.5*contrast)*max(0.0,nc)*max(0.0,tc)/(1.0+distance/(radius*radius));
}
fn lmReconWrite(offset:u32,value:vec3f){lout[offset]=bitcast<u32>(value.x);lout[offset+1u]=bitcast<u32>(value.y);lout[offset+2u]=bitcast<u32>(value.z);}
@compute @workgroup_size(64)
fn lm_reconstruct(@builtin(global_invocation_id) invocation:vec3u){
 let index=invocation.x;if index>=lp.count{return;}
 let center=lmReconRecord(lt[lt[6]+index]);
 let output=index*LM_RESULT;
 var means:array<vec3f,4>;var varianceSums:array<vec3f,4>;
 for(var k=0u;k<4u;k++){means[k]=tv(center+16u+k*3u);varianceSums[k]=tv(center+28u+k*3u)/f32(lt[center+11u]);}
 var direct=tv(center+40u);var env=tv(center+43u);var reflected=tv(center+46u);
 var total=1.0;
 for(var edge=0u;edge<lt[center+51u];edge++){
  let link=lt[5]+(lt[center+50u]+edge)*2u;
  let neighbor=lmReconRecord(lt[link]);
  let weight=tf(link+1u)*lmReconWeight(center,neighbor);let nextTotal=total+weight;let blend=weight/nextTotal;total=nextTotal;
  for(var k=0u;k<4u;k++){
   means[k]+= (tv(neighbor+16u+k*3u)-means[k])*blend;
   varianceSums[k]=varianceSums[k]*(1.0-blend)*(1.0-blend)+tv(neighbor+28u+k*3u)*(blend*blend/f32(lt[neighbor+11u]));
  }
  direct+=(tv(neighbor+40u)-direct)*blend;env+=(tv(neighbor+43u)-env)*blend;reflected+=(tv(neighbor+46u)-reflected)*blend;
 }
 let strength=tf(13u);let retained=1.0-strength;
 var rawValid=true;var stableValid=true;var rawValues:array<vec3f,4>;
 for(var k=0u;k<4u;k++){
  means[k]=mix(tv(center+16u+k*3u),means[k],strength);
  let centerVariance=tv(center+28u+k*3u)/f32(lt[center+11u]);
  let meanVariance=strength*strength*varianceSums[k]+(retained*retained+2.0*retained*strength/total)*centerVariance;
  lmReconWrite(output+k*3u,means[k]);
  let sampleVariance=meanVariance*f32(lt[center+11u]);
  lmReconWrite(output+43u+k*3u,sampleVariance);
  if !all(sampleVariance>=vec3f(0.0))||!all(sampleVariance<=vec3f(LM_FAR)){stableValid=false;}
  let varianceTerm=sampleVariance*(f32(lt[center+11u]-1u)/f32(lt[center+11u]));
  if any(means[k]>vec3f(1.8446743e19)){rawValid=false;}else{
   let square=means[k]*means[k];
   if any(varianceTerm>vec3f(LM_FAR)-square){rawValid=false;}else{rawValues[k]=square+varianceTerm;}
  }
 }
 lmReconWrite(output+24u,mix(tv(center+40u),direct,strength));
 lmReconWrite(output+27u,mix(tv(center+43u),env,strength));
 lmReconWrite(output+30u,mix(tv(center+46u),reflected,strength));
 lout[output+33u]=select(0u,lt[center+11u],stableValid);
 for(var k=0u;k<4u;k++){lmReconWrite(output+12u+k*3u,select(vec3f(0.0),rawValues[k],rawValid));}
 lout[output+55u]=select(0u,1u,stableValid)|select(0u,2u,rawValid);
 var coefficients:array<vec3f,3>;
 for(var channel=0u;channel<3u;channel++){
  let projected=lmProjectRNM(means[0][channel],vec3f(means[1][channel],means[2][channel],means[3][channel]));
  for(var basis=0u;basis<3u;basis++){coefficients[basis][channel]=projected[basis];}
 }
 for(var basis=0u;basis<3u;basis++){lmReconWrite(output+34u+basis*3u,coefficients[basis]);}
}
