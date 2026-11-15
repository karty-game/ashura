//go:build !js

package lightmapper

import (
	"fmt"
	"math"
)

const gpuCoverageTolerance = 1e-4
const gpuUVDeterminantMinimum = 1e-20
const gpuPolygonFanBase = 2

func gpuTileWordCount(receivers []Receiver) (uint64, error) {
	count := uint64(gpuTileHeaderWords) + uint64(len(receivers))*gpuReceiverWords
	for _, receiver := range receivers {
		if len(receiver.Patches) > 0 {
			count += uint64(len(receiver.Patches)) * gpuPatchWords
		} else {
			for _, piece := range receiver.Pieces {
				if len(piece.Barycentric) < gpuRGBWords {
					return 0, ErrInvalid
				}

				count += (uint64(len(piece.Barycentric)) - gpuPolygonFanBase) * gpuPatchWords
			}
		}

		if count*gpuWordBytes > gpuMaximumBytes {
			return 0, ErrBounds
		}
	}

	return count, nil
}

func gpuPackReceiver(words gpuWords, scene *PreparedScene, receiver Receiver, offset int) (gpuWords, error) {
	if receiver.Side != 1 && receiver.Side != -1 || !finite(receiver.Coverage) || receiver.Coverage <= 0 {
		return nil, ErrInvalid
	}

	patches, err := ReceiverPatches(scene, receiver)
	if err != nil {
		return nil, err
	}

	if len(patches) == 0 {
		return nil, ErrInvalid
	}

	words[offset] = uint32(receiver.ID & math.MaxUint32)
	words[offset+1] = uint32(receiver.ID >> 32)
	words[offset+2] = receiver.Pixel
	words[offset+3] = receiver.Chart
	words[offset+4] = words[3]
	words[offset+5] = gpuOffsetWord(len(patches))
	words[offset+6] = gpuSideBits(receiver.Side)
	words[offset+7] = math.Float32bits(receiver.Coverage)

	var area float32

	for _, patch := range patches {
		words, err = gpuPackPatch(words, patch)
		if err != nil {
			return nil, err
		}

		area += patch.Area
	}

	if !finite(area) || float32(math.Abs(float64(area-receiver.Coverage))) > gpuCoverageTolerance*area {
		return nil, fmt.Errorf("receiver coverage mismatch: %w", ErrInvalid)
	}

	return words, nil
}

func gpuPackPatch(words gpuWords, patch ReceiverPatch) (gpuWords, error) {
	if !finite(patch.Area) || patch.Area <= 0 {
		return nil, ErrInvalid
	}

	offset := words.reserve(gpuPatchWords)

	for corner := range 3 {
		if !validVec(patch.Positions[corner]) || !validVec(patch.Normals[corner]) {
			return nil, ErrInvalid
		}

		words.vec(offset+corner*4, patch.Positions[corner])
		words.vec(offset+12+corner*4, patch.Normals[corner])
	}

	words[offset+3] = math.Float32bits(patch.Area)
	words[offset+7] = patch.Triangle
	words[offset+11] = patch.Material

	frame, err := gpuPatchFrame(patch)
	if err != nil {
		return nil, err
	}

	words.vec(offset+28, frame[0])
	words.vec(offset+32, frame[1])
	words.vec(offset+36, frame[2])

	for axis, component := range []float32{patch.UV[0][0], patch.UV[0][1], patch.UV[1][0], patch.UV[1][1], patch.UV[2][0], patch.UV[2][1]} {
		if !finite(component) {
			return nil, ErrInvalid
		}

		words[offset+[6]int{15, 19, 23, 24, 25, 26}[axis]] = math.Float32bits(component)
	}

	words[3]++
	if len(words)*gpuWordBytes > gpuMaximumBytes {
		return nil, ErrBounds
	}

	return words, nil
}

func gpuPatchFrame(patch ReceiverPatch) ([3]Vec3, error) {
	geometric := patch.GeometricNormal
	if geometric == (Vec3{}) {
		geometric = Normalize(Cross(Sub(patch.Positions[1], patch.Positions[0]), Sub(patch.Positions[2], patch.Positions[0])))
	}

	rawTangent, rawBitangent := patch.RawTangent, patch.RawBitangent
	if rawTangent == (Vec3{}) {
		deltaA, deltaB := Sub(patch.Positions[1], patch.Positions[0]), Sub(patch.Positions[2], patch.Positions[0])
		uvA := Vec2{patch.UV[1][0] - patch.UV[0][0], patch.UV[1][1] - patch.UV[0][1]}
		uvB := Vec2{patch.UV[2][0] - patch.UV[0][0], patch.UV[2][1] - patch.UV[0][1]}

		determinant := uvA[0]*uvB[1] - uvA[1]*uvB[0]
		if float32(math.Abs(float64(determinant))) > gpuUVDeterminantMinimum {
			rawTangent = Scale(Sub(Scale(deltaA, uvB[1]), Scale(deltaB, uvA[1])), 1/determinant)
			rawBitangent = Scale(Sub(Scale(deltaB, uvA[0]), Scale(deltaA, uvB[0])), 1/determinant)
		}
	}

	if geometric == (Vec3{}) || !validVec(geometric) || !validVec(rawTangent) || !validVec(rawBitangent) {
		return [3]Vec3{}, ErrInvalid
	}

	return [3]Vec3{geometric, rawTangent, rawBitangent}, nil
}
