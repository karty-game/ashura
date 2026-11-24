//go:build !js

package levelbuild

import (
	"encoding/binary"
	"maps"
	"math"

	"github.com/karty-game/ashura/internal/modeldata"
	"github.com/karty-game/ashura/pkg/assetbuild"
)

// Independent receiver sides are ordinary offline geometry, with separate UVs
// and lighting. Visibility geometry remains the original complete scene.
//
//nolint:mnd // Reversing 32-byte interleaved vertices uses the established 12-byte normal offset.
func independentReceiverSides(
	model modeldata.Document,
	sourceBumps map[string]assetbuild.Artifact,
) (modeldata.Document, map[string]assetbuild.Artifact) {
	result := model
	result.Parts = nil
	bumps := make(map[string]assetbuild.Artifact, len(sourceBumps))
	maps.Copy(bumps, sourceBumps)

	for _, original := range model.Parts {
		front := original
		front.Material.DoubleSided = false
		result.Parts = append(result.Parts, front)

		if !original.Material.DoubleSided {
			continue
		}

		back := front
		back.Name += "@back"

		back.Vertices = append([]byte(nil), original.Vertices...)

		back.Indices = append([]byte(nil), original.Indices...)
		for offset := 0; offset < len(back.Vertices); offset += 32 {
			for axis := range 3 {
				at := offset + 12 + axis*4
				normal := math.Float32frombits(binary.LittleEndian.Uint32(back.Vertices[at:]))
				binary.LittleEndian.PutUint32(back.Vertices[at:], math.Float32bits(-normal))
			}
		}

		for offset := 0; offset < len(back.Indices); offset += 12 {
			a := binary.LittleEndian.Uint32(back.Indices[offset:])
			b := binary.LittleEndian.Uint32(back.Indices[offset+8:])
			binary.LittleEndian.PutUint32(back.Indices[offset:], b)
			binary.LittleEndian.PutUint32(back.Indices[offset+8:], a)
		}

		if bump, ok := bumps[original.Name]; ok {
			bumps[back.Name] = bump
		}

		result.Parts = append(result.Parts, back)
	}

	return result, bumps
}
