//go:build !js

package lightmapper

import "math"

const gpuRGBWords = 3
const gpuReceiverWords = 8
const gpuTileHeaderWords = 8
const gpuSceneVersion = 5
const gpuWideDepthWord = 36
const gpuWideModeWord = 37
const gpuWideMaximumDepth = 11
const gpuWideLeafPrimitives = 3
const gpuAnalyticCountWord = 35
const gpuEmitterCountWord = 13

func gpuEmitterSlot(index int32) uint32 {
	if index < 0 {
		return 0
	}

	return uint32(index) + 1
}
func gpuSideBits(side int32) uint32 {
	if side < 0 {
		return math.MaxUint32
	}

	return 1
}

func gpuSceneWordCount(scene *PreparedScene) uint64 {
	source := scene.source
	entries := []struct{ count, stride uint64 }{
		{uint64(len(scene.order)), 12},
		{uint64(len(source.Triangles)), gpuTriangleWords},
		{uint64(len(source.Materials)), gpuMaterialWords},
		{uint64(len(source.Textures)), 4},
		{uint64(len(source.Emitters)), gpuEmitterWords},
		{uint64(len(scene.lightNodes)), gpuLightNodeWords},
		{uint64(len(scene.lightOrder)), 1},
		{uint64(len(scene.lightLeaves)), 1},
		{uint64(len(scene.lightParents)), 1},
		{uint64(len(scene.environmentRows)), 1},
		{uint64(len(scene.environmentColumns)), 1},
	}

	count := uint64(gpuSceneHeaderWords + 3)
	for _, entry := range entries {
		count += entry.count * entry.stride
	}

	for _, texture := range source.Textures {
		count += uint64(len(texture.Pixels)) * gpuRGBWords
	}

	return count
}

func gpuPackSceneHeader(words gpuWords, scene *PreparedScene) gpuWords {
	source := scene.source
	words[0] = 0x4b4c4d31
	words[1] = gpuSceneVersion
	words[19] = math.Float32bits(source.MetresPerUnit)
	words[20] = math.Float32bits(source.Environment.Sky[0])
	words[21] = math.Float32bits(source.Environment.Sky[1])
	words[22] = math.Float32bits(source.Environment.Sky[2])
	words.vec(23, source.Environment.Ground)

	words[26] = source.Environment.Texture
	if source.Environment.LegacyIrradiance {
		words[27] = 1
	}

	words[28] = gpuEmitterSlot(scene.environmentEmitter)

	for _, emitter := range source.Emitters {
		if emitter.Kind == Rectangle || emitter.Kind == Disk || emitter.Kind == Sphere {
			words[gpuAnalyticCountWord]++
		}
	}

	return words
}

func gpuPackSceneTriangles(words gpuWords, scene *PreparedScene) gpuWords {
	source := scene.source
	words[6] = gpuOffsetWord(len(words))

	words[5] = gpuOffsetWord(len(source.Triangles))
	for index, entry := range source.Triangles {
		offset := words.reserve(gpuTriangleWords)
		words.vec(offset, entry.Positions[0])
		words[offset+3] = entry.Material
		words.vec(offset+4, entry.Positions[1])
		words[offset+7] = gpuEmitterSlot(scene.triangleEmitters[index])
		words.vec(offset+8, entry.Positions[2])
		words[offset+11] = uint32(entry.ID & math.MaxUint32)
		words.vec(offset+12, entry.Normals[0])
		words[offset+15] = uint32(entry.ID >> 32)
		words.vec(offset+16, entry.Normals[1])
		words.vec(offset+20, entry.Normals[2])
		words[offset+19] = math.Float32bits(entry.UV[0][0])

		words[offset+23] = math.Float32bits(entry.UV[0][1])
		cross := Cross(Sub(entry.Positions[1], entry.Positions[0]), Sub(entry.Positions[2], entry.Positions[0]))
		words.vec(offset+28, cross)
		words[offset+31] = math.Float32bits(Length(cross))

		for j, component := range []float32{entry.UV[1][0], entry.UV[1][1], entry.UV[2][0], entry.UV[2][1]} {
			words[offset+24+j] = math.Float32bits(component)
		}
	}

	return words
}

func gpuPackSceneMaterials(words gpuWords, scene *PreparedScene) gpuWords {
	source := scene.source
	words[8] = gpuOffsetWord(len(words))

	words[7] = gpuOffsetWord(len(source.Materials))
	for _, material := range source.Materials {
		offset := words.reserve(gpuMaterialWords)
		words.vec(offset, material.Reflectance)
		words[offset+3] = material.Texture
		words.vec(offset+4, material.Emission)

		if material.DoubleSided {
			words[offset+7] = 1
		}
	}

	return words
}

func gpuPackSceneTextures(words gpuWords, scene *PreparedScene) gpuWords {
	source := scene.source
	words[10] = gpuOffsetWord(len(words))
	words[9] = gpuOffsetWord(len(source.Textures))

	textures := words.reserve(len(source.Textures) * 4)
	for index, entry := range source.Textures {
		words[textures+index*4] = entry.Width
		words[textures+index*4+1] = entry.Height

		words[textures+index*4+2] = gpuOffsetWord(len(words))
		for _, pixel := range entry.Pixels {
			offset := words.reserve(3)
			words.vec(offset, pixel)
		}
	}

	return words
}

func gpuPackSceneEmitters(words gpuWords, scene *PreparedScene) gpuWords {
	source := scene.source
	words[12] = gpuOffsetWord(len(words))

	words[13] = gpuOffsetWord(len(source.Emitters))
	for index, emitter := range source.Emitters {
		offset := words.reserve(gpuEmitterWords)
		words[offset] = uint32(emitter.Kind)

		words[offset+1] = uint32(emitter.Model)
		if emitter.TwoSided {
			words[offset+2] = 1
		}

		words[offset+3] = emitter.Triangle
		words.vec(offset+4, emitter.Position)
		words[offset+7] = math.Float32bits(emitter.Radius)
		words.vec(offset+8, emitter.Direction)
		words[offset+11] = math.Float32bits(emitter.Range)
		words.vec(offset+12, emitter.Tangent)
		words[offset+15] = math.Float32bits(emitter.InnerCos)
		words.vec(offset+16, emitter.Intensity)
		words[offset+19] = math.Float32bits(emitter.OuterCos)
		words[offset+20] = math.Float32bits(emitter.Size[0])
		words[offset+21] = math.Float32bits(emitter.Size[1])
		words[offset+22] = scene.lightLeaves[index]
	}

	return words
}

func gpuPackSceneLights(words gpuWords, scene *PreparedScene) gpuWords {
	words[14] = gpuOffsetWord(len(words))

	words[15] = gpuOffsetWord(len(scene.lightNodes))
	for _, node := range scene.lightNodes {
		offset := words.reserve(gpuLightNodeWords)
		words.vec(offset, node.Min)
		words[offset+3] = node.Left
		words.vec(offset+4, node.Max)
		words[offset+7] = node.Right
		words[offset+8] = node.First
		words[offset+9] = node.Count
		words[offset+10] = math.Float32bits(node.Power)
		words[offset+11] = math.Float32bits(node.GlobalPower)
		words.vec(offset+12, node.Direction)
		words[offset+15] = math.Float32bits(node.ConeCos)
	}

	words[16] = gpuOffsetWord(len(words))
	words = append(words, scene.lightOrder...)
	words[17] = gpuOffsetWord(len(words))
	words = append(words, scene.lightLeaves...)
	words[18] = gpuOffsetWord(len(words))
	words = append(words, scene.lightParents...)

	return words
}

func gpuPackSceneEnvironment(words gpuWords, scene *PreparedScene) gpuWords {
	words[29] = gpuOffsetWord(len(words))

	words[30] = gpuOffsetWord(len(scene.environmentRows))
	for _, component := range scene.environmentRows {
		words = append(words, math.Float32bits(component))
	}

	words[31] = gpuOffsetWord(len(words))

	words[32] = gpuOffsetWord(len(scene.environmentColumns))
	for _, component := range scene.environmentColumns {
		words = append(words, math.Float32bits(component))
	}

	return words
}

//nolint:gosec // G115: Every caller is a packing path preflight-capped at128MiB; all counts/offsets fit uint32.
func gpuOffsetWord(value int) uint32 { return uint32(value) }

func gpuPackRayTriangles(words gpuWords, scene *PreparedScene, order []uint32) gpuWords {
	// The readonly vec4 scene alias requires a complete aligned final AS.
	const vectorWords = 4

	words.reserve((vectorWords - len(words)%vectorWords) % vectorWords)
	words[33] = gpuOffsetWord(len(words))
	original := gpuGeometryPriorities(scene)

	priorities := make([]uint32, len(scene.order))
	for slot, triangle := range scene.order {
		priorities[triangle] = original[slot]
	}

	for _, triangle := range order {
		entry := scene.source.Triangles[triangle]
		edge1 := Sub(entry.Positions[1], entry.Positions[0])
		edge2 := Sub(entry.Positions[2], entry.Positions[0])
		offset := words.reserve(12)
		words.vec(offset, entry.Positions[0])
		words[offset+3] = math.Float32bits(Length(Cross(edge1, edge2)))
		words.vec(offset+4, edge1)
		words[offset+7] = triangle
		words.vec(offset+8, edge2)
		words[offset+11] = priorities[triangle]
	}

	return words
}
