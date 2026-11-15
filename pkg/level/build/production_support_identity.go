//go:build !js

package levelbuild

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"

	"github.com/karty-game/karty-webgpu/pkg/assetbuild"
	bake "github.com/karty-game/karty-webgpu/pkg/lightmap/bake"
)

// Production support depends on exact ordered immutable receiver/geometry
// artifacts, not lighting. Their admitted SHA256s already bind every patch,
// frame, pixel, side and original material index. GeometryHash also binds the
// complete scene and its material remapping. Re-encoding a giant receiver JSON
// array would repeat those same integrity checks without adding provenance.
func productionSupportIdentity(geometryHash string, pages []pageRecord) (string, error) {
	geometry, err := hex.DecodeString(geometryHash)
	if err != nil || len(geometry) != sha256.Size {
		return "", ErrInvalid
	}

	digest := sha256.New()
	_, _ = digest.Write([]byte("karty.surface-support-page-inputs@1"))
	_, _ = digest.Write(geometry)

	var record [16]byte
	binary.LittleEndian.PutUint64(record[:8], uint64(len(pages)))
	_, _ = digest.Write(record[:8])

	for _, page := range pages {
		for _, artifact := range []assetbuild.Artifact{page.Receivers, page.Geometry} {
			hash, decodeErr := hex.DecodeString(artifact.SHA256)
			if decodeErr != nil || len(hash) != sha256.Size || artifact.Size < 0 {
				return "", ErrInvalid
			}

			_, _ = digest.Write(hash)

			binary.LittleEndian.PutUint64(record[:8], uint64(artifact.Size))
			_, _ = digest.Write(record[:8])
		}

		binary.LittleEndian.PutUint32(record[:4], page.Width)
		binary.LittleEndian.PutUint32(record[4:8], page.Height)
		_, _ = digest.Write(record[:8])
	}

	return hex.EncodeToString(digest.Sum(nil)), nil
}

func supportEdgeBefore(first, second bake.ReconstructionEdge) bool {
	return first.From < second.From || first.From == second.From && first.To < second.To
}
