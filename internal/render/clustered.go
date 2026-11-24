package render

import (
	_ "embed"
	"math"
	"strings"
)

const (
	// Leave room for the frame and shadow matrices in a 16 KiB uniform binding.
	ClusterLightLimit   = 240
	ClusterSlices       = 24
	clusterTileLimit    = 96
	clusterTileSize     = 16
	clusterWordBits     = 32
	clusterMaskWords    = (ClusterLightLimit + clusterWordBits - 1) / clusterWordBits
	ClusterMaskBytes    = clusterTileLimit * clusterTileLimit * ClusterSlices * clusterMaskWords * 4
	clusterHeaderFloats = 28
	clusterLightFloats  = 16
	ClusterUniformSize  = (clusterHeaderFloats + ClusterLightLimit*clusterLightFloats + SpotShadowCount*spotMatrixFloats) * 4
)

//go:embed wgsl/cluster-frame.wgsl
var clusterFrameShader string

//go:embed wgsl/cluster-build.wgsl
var clusterBuildShader string

//go:embed wgsl/cluster-lighting.wgsl
var clusterLightingShader string

func ClusterBuildShader() string { return clusterFrameShader + clusterBuildShader }

// ClusterMaterialShader specializes the light loop while preserving the material
// evaluation, including its baked irradiance and normal response.
func ClusterMaterialShader(shader string) string {
	const loop = "for (var index = 0u; index < u32(sceneFrame.camera.w); index++) {\n"

	variant := strings.Replace(
		shader,
		loop+"        let light = sceneFrame.lights[index];",
		`let count = u32(clusterFrame.grid.w);
    let allLights = clusterFrame.depth.z != 0.0;
    var maskBase = 0u;
    if count != 0u && !allLights {
        maskBase = fragmentClusterBase(input.position.xy, input.worldPosition);
    }
    for (var word = 0u; word < clusterWords(count); word++) {
      var indices = lightWord(maskBase, word, count, allLights);
      while indices != 0u {
        let index = word * clusterWordBits + firstTrailingBit(indices);
        indices &= indices - 1u;
        let light = clusterFrame.lights[index];
        if light.color.w == 0.0 { continue; }`,
		1,
	)
	variant = strings.Replace(variant, "    let visibility = pointVisibility(input.worldPosition);\n", "", 1)
	variant = strings.Replace(variant, "if light.direction.w == 3.0 {", "if light.direction.w >= 3.0 {", 1)
	variant = strings.Replace(
		variant,
		"if light.ground.y > 0.0 { attenuation *= pointVisibility(input.worldPosition); }",
		"attenuation = localAttenuation(light, input.worldPosition, towardLight, attenuation);\n                if attenuation <= 0.0 { continue; }",
		1,
	)
	variant = strings.Replace(
		variant,
		"if light.ground.y > 0.0 { attenuation *= visibility; }",
		"attenuation = localAttenuation(light, input.worldPosition, towardLight, attenuation);\n            if attenuation <= 0.0 { continue; }",
		1,
	)
	variant = strings.Replace(variant, "    let fresnel =", "    }\n    let fresnel =", 1)
	variant = strings.Replace(variant, "    return lighting;", "    }\n    return lighting;", 1)

	return variant + "\n" + clusterFrameShader + clusterLightingShader
}

type clusterInfluence struct {
	position [3]float32
	radius   float32
	kind     uint32
}

type clusterKey struct {
	projection [16]float32
	depthPlane [4]float32
	ranges     [2]float32
	grid       [4]float32
	influences [ClusterLightLimit]clusterInfluence
}

// ClusterFrameState retains the uploaded uniforms and the culling inputs.
// Radiance, cone rotation and shadow-only changes do not rebuild spherical masks.
type ClusterFrameState struct {
	key    clusterKey
	values [ClusterUniformSize / 4]float32
	Data   [ClusterUniformSize]byte
	Groups [3]uint32
	ready  bool
}

func (state *ClusterFrameState) Invalidate() { state.ready = false }

func (state *ClusterFrameState) Update(frame Frame, width, height uint32, spots [SpotShadowCount]SpotView) (upload, dispatch bool) {
	tile := max(clusterTileSize, divideRoundUp(max(width, height), clusterTileLimit))
	columns, rows := divideRoundUp(width, tile), divideRoundUp(height, tile)
	state.Groups = [3]uint32{columns, rows, 1}

	near, far := frame.CameraNear, frame.CameraFar

	unculled := frame.UnculledLights || near <= 0 || far <= near || !pointFinite(near) || !pointFinite(far)
	if unculled {
		near, far = 1, 2
	}

	key := clusterKey{
		projection: frame.ViewProjection, depthPlane: clusterDepthPlane(frame),
		ranges: [2]float32{near, far}, grid: [4]float32{float32(width), float32(height), float32(tile), 0},
	}

	var values [ClusterUniformSize / 4]float32
	copy(values[:16], key.projection[:])
	copy(values[16:20], key.depthPlane[:])

	values[24], values[25] = near, float32(math.Log2(float64(far)/float64(near)))
	if unculled {
		values[26] = 1
	}

	spotSlot := 0

	for index, light := range frame.Lights {
		offset := clusterHeaderFloats + index*clusterLightFloats

		if light.Kind == SpotLightKind && light.CastShadows {
			if spots[spotSlot].Active {
				values[offset+10] = float32(spotSlot + 1)
			}

			spotSlot++
		}

		if light.Intensity == 0 {
			continue
		}

		key.grid[3] = float32(index + 1)

		packClusterLight(values[offset:offset+clusterLightFloats], light)
		key.influences[index] = clusterInfluence{light.Direction, light.GroundColor[0], light.Kind}
	}

	copy(values[20:24], key.grid[:])

	for slot, view := range spots {
		offset := clusterHeaderFloats + ClusterLightLimit*clusterLightFloats + slot*spotMatrixFloats
		copy(values[offset:offset+spotMatrixFloats], view.Matrix[:])
	}

	dispatch = !unculled && key.grid[3] != 0 && (!state.ready || state.values[26] != 0 || key != state.key)

	upload = !state.ready || values != state.values
	if upload {
		encodeUniform(state.Data[:], values[:])
	}

	state.key, state.values, state.ready = key, values, true

	return upload, dispatch
}

func packClusterLight(values []float32, light Light) {
	copy(values[:3], light.Direction[:])
	values[3] = float32(light.Kind)
	copy(values[4:7], light.Color[:])
	values[7] = light.Intensity
	copy(values[8:10], light.GroundColor[:2])

	if light.Kind != SpotLightKind {
		values[10] = light.GroundColor[2]

		return
	}

	direction := normalized3(light.SpotDirection)
	copy(values[12:15], direction[:])
	values[11], values[15] = cosine(light.InnerAngle), cosine(light.OuterAngle)
}

func divideRoundUp(value, divisor uint32) uint32 { return value/divisor + min(1, value%divisor) }

func clusterDepthPlane(frame Frame) [4]float32 {
	matrix := frame.ViewProjection
	if frame.PerspectiveCamera {
		return [4]float32{matrix[3], matrix[7], matrix[11], matrix[15]}
	}

	span := frame.CameraFar - frame.CameraNear

	return [4]float32{matrix[2] * span, matrix[6] * span, matrix[10] * span, matrix[14]*span + frame.CameraNear}
}
