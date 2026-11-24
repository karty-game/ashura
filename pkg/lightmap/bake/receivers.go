//go:build !js

package lightmapbake

import (
	"encoding/binary"
	"fmt"
	"math"

	"github.com/karty-game/ashura/internal/lightmapper"
	"github.com/karty-game/ashura/internal/modeldata"
)

// PrepareReceivers clips UV1 triangles against texel footprints. The resulting
// world support is independent of the visibility scene's triangulation.
func PrepareReceivers(scene Scene, uv1 [][3]Vec2, width, height uint32) ([]Receiver, error) {
	receivers, err := lightmapper.PrepareReceivers(scene.Triangles, uv1, width, height)
	if err != nil {
		return nil, err
	}

	if err = lightmapper.MaterializeReceiverPatches(scene.Triangles, receivers); err != nil {
		return nil, err
	}

	for i := range receivers {
		for j := range receivers[i].Patches {
			receivers[i].Patches[j].Triangle = math.MaxUint32
		}
	}

	return receivers, nil
}

// ReceiversFromModel binds exact admitted UV1 streams to prepared geometry.
// It takes no ownership of game runtime models or GPU handles.
//
//nolint:gosec,varnamelen // G602: part/UV stream counts and admitted model vertex indices are checked before indexing; uv denotes each triangle’s lightmap coordinates, distinct from the model UV0 stream.
func ReceiversFromModel(data []byte, uvs [][]byte, width, height uint32) ([]Receiver, error) {
	model, err := modeldata.Decode(data)
	if err != nil {
		return nil, err
	}

	if len(model.Parts) != len(uvs) {
		return nil, ErrInvalid
	}

	triangles, err := modelTriangles(model)
	if err != nil {
		return nil, err
	}

	coordinates := make([][3]Vec2, 0, len(triangles))
	for partIndex, part := range model.Parts {
		if len(uvs[partIndex]) != len(part.Vertices)/4 {
			return nil, ErrInvalid
		}

		for offset := 0; offset < len(part.Indices); offset += 12 {
			var uv [3]Vec2

			for corner := range 3 {
				index := int(binary.LittleEndian.Uint32(part.Indices[offset+corner*4:]))
				uv[corner] = Vec2{readNumber(uvs[partIndex][index*8:]), readNumber(uvs[partIndex][index*8+4:])}
			}

			coordinates = append(coordinates, uv)
		}
	}

	receivers, err := PrepareReceivers(Scene{Triangles: triangles}, coordinates, width, height)
	if err != nil {
		return nil, fmt.Errorf("model receivers (%d triangles, %dx%d): %w", len(coordinates), width, height, err)
	}

	return receivers, nil
}
