//go:build !js

package lightmapper

import (
	"context"
	"encoding/binary"
	"math"
)

// PackGPUVerification appends local response normals to the ordinary receiver
// footprint packet. One path sample contributes to every requested normal;
// padded probes carry no additional tracing work and are excluded by decoding.
//
//nolint:gosec // G115: the bounded existing tile byte length is divisible by its four-byte word stride.
func PackGPUVerification(scene *PreparedScene, receivers []Receiver, options Options, normals [][]Vec3) (GPUTile, error) {
	if len(normals) != len(receivers) || len(receivers) == 0 {
		return GPUTile{}, ErrInvalid
	}

	maximum := 0

	for _, receiverNormals := range normals {
		if len(receiverNormals) == 0 {
			return GPUTile{}, ErrInvalid
		}

		maximum = max(maximum, len(receiverNormals))
		for _, normal := range receiverNormals {
			if !validVec(normal) || math.Abs(float64(Dot(normal, normal)-1)) > 1e-3 || normal[2] < 0 {
				return GPUTile{}, ErrInvalid
			}
		}
	}

	if uint64(maximum) > math.MaxUint32 || uint64(maximum)*uint64(len(receivers))*12 > gpuMaximumBytes {
		return GPUTile{}, ErrBounds
	}

	tile, err := PackGPUTile(scene, receivers, options)
	if err != nil {
		return tile, err
	}

	if len(tile.Data) < 32 || uint64(len(tile.Data))+uint64(maximum)*uint64(len(receivers))*12 > gpuMaximumBytes {
		return GPUTile{}, ErrBounds
	}

	offset := uint32(len(tile.Data) / 4)
	binary.LittleEndian.PutUint32(tile.Data[16:], offset)
	binary.LittleEndian.PutUint32(tile.Data[20:], uint32(maximum))

	for _, receiverNormals := range normals {
		for index := range maximum {
			normal := Vec3{0, 0, 1}
			if index < len(receiverNormals) {
				normal = receiverNormals[index]
			}

			for _, value := range normal {
				tile.Data = binary.LittleEndian.AppendUint32(tile.Data, math.Float32bits(value))
			}
		}
	}

	tile.Mode = "verify"
	tile.ProbeCount = uint32(maximum)

	return tile, nil
}

//nolint:mnd // The verification ABI stores nine float channels per probe, four-byte words and two validity flags.
func DecodeGPUResponses(data []byte, receivers []Receiver, normals [][]Vec3, maximum uint32) ([]ResponseEstimate, error) {
	stride := uint64(maximum)*9 + 2
	if maximum == 0 || len(normals) != len(receivers) || uint64(len(receivers))*stride*4 != uint64(len(data)) {
		return nil, ErrInvalid
	}

	result := make([]ResponseEstimate, len(receivers))
	for receiverIndex, receiver := range receivers {
		if uint64(len(normals[receiverIndex])) > uint64(maximum) {
			return nil, ErrInvalid
		}

		offset := uint64(receiverIndex) * stride * 4
		value := ResponseEstimate{
			ReceiverID:    receiver.ID,
			Count:         binary.LittleEndian.Uint32(data[offset+uint64(maximum)*36:]),
			Means:         make([]Vec3, len(normals[receiverIndex])),
			SecondMoments: make([]Vec3, len(normals[receiverIndex])),
		}
		value.Variance = make([]Vec3, len(normals[receiverIndex]))
		flags := binary.LittleEndian.Uint32(data[offset+uint64(maximum)*36+4:])
		value.VarianceValid = flags&1 != 0

		value.SecondMomentsValid = flags&2 != 0
		if value.Count == 0 || (value.Count > 1 && !value.VarianceValid) || flags&^uint32(3) != 0 {
			return nil, ErrInvalid
		}

		for probe := range normals[receiverIndex] {
			for channel := range 3 {
				value.Means[probe][channel] = math.Float32frombits(binary.LittleEndian.Uint32(data[offset+uint64(probe*3+channel)*4:]))
				value.Variance[probe][channel] = math.Float32frombits(
					binary.LittleEndian.Uint32(data[offset+uint64(maximum)*24+uint64(probe*3+channel)*4:]),
				)
				value.SecondMoments[probe][channel] = math.Float32frombits(
					binary.LittleEndian.Uint32(data[offset+uint64(maximum)*12+uint64(probe*3+channel)*4:]),
				)
			}

			if !positiveVec(value.Means[probe]) || !positiveVec(value.Variance[probe]) ||
				(!value.VarianceValid && value.Variance[probe] != (Vec3{})) ||
				(value.SecondMomentsValid && !positiveVec(value.SecondMoments[probe])) {
				return nil, ErrInvalid
			}
		}

		result[receiverIndex] = value
	}

	return result, nil
}

// BakeResponses is the independent full-response hardware estimator used by
// VerifyRNM. Memory-driven receiver tiling preserves IDs, seed and sample count.
func (worker *GPUWorker) BakeResponses(
	ctx context.Context,
	scene *PreparedScene,
	receivers []Receiver,
	options Options,
	normals [][]Vec3,
) ([]ResponseEstimate, error) {
	return worker.bakeSeededResponses(ctx, scene, verificationGPURequest{receivers: receivers, normals: normals, options: options})
}

func gpuResponseCountMatches(estimates []ResponseEstimate, count uint32) bool {
	for _, estimate := range estimates {
		if estimate.Count != count {
			return false
		}
	}

	return true
}
