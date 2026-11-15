package lightmapbuild

import (
	"time"

	"github.com/karty-game/karty-webgpu/pkg/texture"
)

// RNMResult contains three one-mip irradiance coefficient atlases. Encoding
// distinguishes normalized linear v2 output from RGBE prepared-transport output.
// Duration is executor wall time. One-shot execution includes setup/compilation;
// an RNMSession job excludes shared startup and includes upload/bake/readback.
type RNMResult struct {
	Textures         [3]texture.Data
	Scale            float32
	Encoding         texture.RNMEncoding
	Adapter          string
	Duration         time.Duration
	Samples, Bounces uint32
}
