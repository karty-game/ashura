package render

import (
	_ "embed"
	"encoding/binary"
	"strings"

	"github.com/karty-game/ashura/internal/decaldata"
	"github.com/karty-game/ashura/internal/texturedata"
)

const (
	MaxDecals        = 32
	DecalHeaderBytes = 16
	DecalRecordBytes = decaldata.RecordBytes
	DecalUniformSize = DecalHeaderBytes + MaxDecals*DecalRecordBytes
)

func (draw DrawCall) DecalVariant() int {
	variant := 0
	if draw.Source {
		variant += 2
	}

	if draw.CullBackfaces {
		variant++
	}

	return variant
}

// DecalFrame contains retained, ordered projection records independent of the view.
type DecalFrame struct {
	Atlas    *texturedata.Texture
	Count    int
	Revision uint64
	Records  [MaxDecals][DecalRecordBytes]byte
	Static   *StaticDecalFrame
}

// DecalUploads compares GPU slots only when the world's retained data changes.
// A newly allocated GPU buffer starts zeroed, including the active count.
type DecalUploads struct {
	frame    *DecalFrame
	revision uint64
	count    int
	records  [MaxDecals][DecalRecordBytes]byte
	ready    bool
}

func (state *DecalUploads) Update(frame *DecalFrame, write func(int, []byte) error) error {
	if state.ready && state.frame == frame && (frame == nil || frame.Revision == state.revision) {
		return nil
	}

	count := 0
	if frame != nil {
		count = frame.Count
	}

	if count != state.count {
		var header [DecalHeaderBytes]byte
		binary.LittleEndian.PutUint32(header[:], uint32(count))

		if err := write(0, header[:]); err != nil {
			state.ready = false

			return err
		}

		state.count = count
	}

	for index := range count {
		record := frame.Records[index]
		if state.records[index] == record {
			continue
		}

		if err := write(DecalHeaderBytes+index*DecalRecordBytes, record[:]); err != nil {
			state.ready = false

			return err
		}

		state.records[index] = record
	}

	state.frame, state.ready = frame, true
	if frame != nil {
		state.revision = frame.Revision
	}

	return nil
}

//go:embed wgsl/decals.wgsl
var decalsShader string

//go:embed wgsl/decal-structs.wgsl
var decalStructs string

//go:embed wgsl/decal-tiles.wgsl
var decalTiles string

func DecalTileShader() string { return decalStructs + "\n" + decalTiles }

// DecalMaterialShader adds projection to the registered forward material variant.
// Group 2 is available unless clustered lights use it.
func DecalMaterialShader(shader string, clustered bool) string {
	shader = strings.Replace(shader, "}\nstruct FrameUniforms", "    decalLayers: vec4u,\n}\nstruct FrameUniforms", 1)
	shader = strings.Replace(shader, "let shadedBase = base;", "let shadedBase = projectDecals("+
		"base, input.worldPosition, select(-input.normal, input.normal, frontFacing), material.decalLayers.x, input.position.xy);", 1)

	projection := decalsShader
	if clustered {
		projection = strings.ReplaceAll(projection, "@group(2)", "@group(3)")
	}

	return shader + "\n" + decalStructs + "\n" + projection
}
