//go:build !js

package lightmapper

import (
	"encoding/binary"
	"errors"
)

const (
	gpuModeVerify              = "verify"
	gpuModeReconstruct         = "reconstruct"
	gpuWordBytes               = 4
	gpuPathHeaderBytes         = 16
	gpuTraceCounterBytes       = 4 * gpuWordBytes
	gpuPathMinimumBytes        = gpuPathHeaderBytes + gpuWordBytes
	gpuBatchSplit              = 2
	gpuPathQueueCount          = 2
	gpuVerificationMomentWords = 9
	gpuVerificationStatusWords = 2
)

var (
	errGPUExecution  = errors.New("lightmapper: native hardware compute execution failed")
	errGPUWorkerBusy = errors.New("lightmapper: hardware worker has an active batch")
)

func gpuValidatePackets(scene GPUScene, tile GPUTile) error {
	if scene.Key == "" || len(scene.Key) > 128 || len(scene.Data) < gpuSceneHeaderWords*gpuWordBytes || len(scene.Data)%16 != 0 {
		return ErrInvalid
	}

	if binary.LittleEndian.Uint32(scene.Data) != 0x4b4c4d31 ||
		binary.LittleEndian.Uint32(scene.Data[gpuWordBytes:]) != gpuSceneVersion ||
		binary.LittleEndian.Uint32(
			scene.Data[gpuAnalyticCountWord*gpuWordBytes:],
		) > binary.LittleEndian.Uint32(
			scene.Data[gpuEmitterCountWord*gpuWordBytes:],
		) || !gpuSceneAligned(scene.Data) {
		return ErrInvalid
	}

	if len(tile.Data) < 16 || len(tile.Data)%gpuWordBytes != 0 || tile.Count == 0 || tile.Options.Bounces > 32 {
		return ErrInvalid
	}

	if tile.Options.Samples == 0 || tile.Options.Samples > 1<<24 {
		return ErrInvalid
	}

	if tile.Mode != "" && tile.Mode != gpuModeVerify && tile.Mode != gpuModeReconstruct {
		return ErrInvalid
	}

	if tile.Mode == gpuModeVerify {
		return gpuValidateVerification(tile)
	}

	return nil
}
