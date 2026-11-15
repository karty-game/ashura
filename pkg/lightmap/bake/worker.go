//go:build !js

package lightmapbake

import (
	"context"

	"github.com/karty-game/karty-webgpu/internal/lightmapper"
)

type Worker = lightmapper.GPUWorker
type WorkerOptions = lightmapper.GPUWorkerOptions
type Device = lightmapper.GPUDevice
type TraceCounts = lightmapper.GPUTraceCounts
type BakeBatch = lightmapper.BakeBatch

// NewWorker opens a reusable native compute executor, requiring hardware by
// default. SoftwareForCI explicitly selects Vulkan CPU execution for functional
// CI, never local graphics or GPU performance evidence. Its memory ceiling
// covers the complete live allocation set, independently of quality.
func NewWorker(ctx context.Context, options WorkerOptions) (*Worker, error) {
	return lightmapper.NewGPUWorker(ctx, options)
}
