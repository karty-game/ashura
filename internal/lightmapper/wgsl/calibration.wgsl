// Named rnm-flat-30x8-60x8-3to1@1 projection. This file is build-only.
const lmProbeNormals = array<vec3f,16>(
vec3f(0.5,0.0,0.8660254038),
vec3f(0.3535533906,0.3535533906,0.8660254038),
vec3f(3.061616998e-17,0.5,0.8660254038),
vec3f(-0.3535533906,0.3535533906,0.8660254038),
vec3f(-0.5,6.123233996e-17,0.8660254038),
vec3f(-0.3535533906,-0.3535533906,0.8660254038),
vec3f(-9.184850994e-17,-0.5,0.8660254038),
vec3f(0.3535533906,-0.3535533906,0.8660254038),
vec3f(0.8660254038,0.0,0.5),
vec3f(0.6123724357,0.6123724357,0.5),
vec3f(5.302876194e-17,0.8660254038,0.5),
vec3f(-0.6123724357,0.6123724357,0.5),
vec3f(-0.8660254038,1.060575239e-16,0.5),
vec3f(-0.6123724357,-0.6123724357,0.5),
vec3f(-1.590862858e-16,-0.8660254038,0.5),
vec3f(0.6123724357,-0.6123724357,0.5)
);
const lmProbeFitWeights = array<vec3f,16>(
vec3f(0.3093431089,0.03282844554,0.03282844554),
vec3f(0.2332531755,0.1375601184,0.004186706132),
vec3f(0.09375,0.2732075215,0.008042478528),
vec3f(0.01674682453,0.2999398816,0.05831329387),
vec3f(0.003156891076,0.1859215545,0.1859215545),
vec3f(0.01674682453,0.05831329387,0.2999398816),
vec3f(0.09375,0.008042478528,0.2732075215),
vec3f(0.2332531755,0.004186706132,0.1375601184),
vec3f(0.125,0.0,0.0),
vec3f(0.09206779562,0.03293220438,0.0),
vec3f(0.01163587275,0.1133641273,0.0),
vec3f(0.0,0.1235391943,0.00146080566),
vec3f(0.0,0.0625,0.0625),
vec3f(0.0,0.00146080566,0.1235391943),
vec3f(0.01163587275,0.0,0.1133641273),
vec3f(0.09206779562,0.0,0.03293220438)
);
const lmFitH = mat3x3f(vec3f(0.8565399604,0.2379336882,0.2379336882),vec3f(0.2379336882,0.8586001754,0.2372624681),vec3f(0.2379336882,0.2372624681,0.8586001754));
fn lmCalibrationMoments(direction: vec3f) -> vec4f {
    var result=vec4f(max(direction.z,0.0)/3.141592653589793,0.0,0.0,0.0);
    for (var j=0u;j<16u;j++) {
        let response=max(dot(lmProbeNormals[j],direction),0.0)/3.141592653589793;
        result=vec4f(result.x,result.yzw+lmProbeFitWeights[j]*response);
    }
    return result;
}
fn lmProjectionScore(c: vec3f,g: vec3f) -> f32 { return dot(c,lmFitH*c)-2.0*dot(g,c); }
fn lmProjectRNMNormalized(flat: f32,g: vec3f) -> vec3f {
    let total=3.0*max(flat,0.0);
    if total<=0.0{return vec3f(0.0);}
    // A feasible equality-constrained minimum is already the simplex optimum.
    // Otherwise the three clamped edge minima include every vertex too.
    let a=lmFitH[0][0]+lmFitH[2][2]-2.0*lmFitH[0][2];
    let b=lmFitH[0][1]-lmFitH[0][2]-lmFitH[1][2]+lmFitH[2][2];
    let d=lmFitH[1][1]+lmFitH[2][2]-2.0*lmFitH[1][2];
    let r0=g.x-g.z-total*(lmFitH[0][2]-lmFitH[2][2]);
    let r1=g.y-g.z-total*(lmFitH[1][2]-lmFitH[2][2]);
    let determinant=a*d-b*b;
    let solutionX=(d*r0-b*r1)/determinant;
    let solutionY=(a*r1-b*r0)/determinant;
    let solution=vec3f(solutionX,solutionY,total-solutionX-solutionY);
    if all(solution>=vec3f(0.0)){return solution;}
    var best=vec3f(total,0.0,0.0);
    var score=lmProjectionScore(best,g);
    for (var i=0u;i<3u;i++) {
        for (var j=i+1u;j<3u;j++) {
            let denominator=lmFitH[i][i]+lmFitH[j][j]-2.0*lmFitH[i][j];
            let x=clamp((g[i]-g[j]-total*(lmFitH[i][j]-lmFitH[j][j]))/denominator,0.0,total);
            var c=vec3f(0.0);c[i]=x;c[j]=total-x;
            let next=lmProjectionScore(c,g);
            if next<score {best=c;score=next;}
        }
    }
    return best;
}

// Homogeneous normalization keeps the NNLS objective finite for valid HDR.
fn lmProjectRNM(flat:f32,g:vec3f)->vec3f{
 let scale=max(flat,max(g.x,max(g.y,g.z)));
 if scale<=0.0{return vec3f(0.0);}
 let value=lmProjectRNMNormalized(flat/scale,g/scale);
 if scale>1.0{if any(value>vec3f(3.402823466e38/scale)){return vec3f(-1.0);}}
 return value*scale;
}
