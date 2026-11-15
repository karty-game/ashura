package lightmapper

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"hash"
)

func identityArray(digest hash.Hash, values any, count int) error {
	if count < 0 || count > maxSceneArtifact {
		return ErrBounds
	}

	if err := binary.Write(digest, binary.LittleEndian, uint32(count)); err != nil {
		return err
	}

	return binary.Write(digest, binary.LittleEndian, values)
}

func (p *PreparedScene) finishSceneIdentity() error {
	if p.geometryHash == "" {
		geometry := p.source
		geometry.Emitters = nil
		geometry.Environment = Environment{}

		encoded, err := encodeSceneSource(geometry)
		if err != nil {
			return err
		}

		digest := sha256.New()
		_, _ = digest.Write([]byte(SceneSchema))

		_, _ = digest.Write(encoded)
		if err = identityArray(digest, p.nodes, len(p.nodes)); err != nil {
			return err
		}

		if err = identityArray(digest, p.order, len(p.order)); err != nil {
			return err
		}

		p.geometryHash = hex.EncodeToString(digest.Sum(nil))
	}

	digest := sha256.New()
	_, _ = digest.Write([]byte(SceneSchema))

	_, _ = digest.Write([]byte(p.geometryHash))
	if err := binary.Write(digest, binary.LittleEndian, p.source.Environment); err != nil {
		return err
	}

	for _, record := range []struct {
		data  any
		count int
	}{{p.source.Emitters, len(p.source.Emitters)}, {p.lightNodes, len(p.lightNodes)}, {p.lightOrder, len(p.lightOrder)}, {p.lightLeaves, len(p.lightLeaves)}, {p.lightParents, len(p.lightParents)}, {p.triangleEmitters, len(p.triangleEmitters)}, {p.environmentRows, len(p.environmentRows)}, {p.environmentColumns, len(p.environmentColumns)}} {
		if err := identityArray(digest, record.data, record.count); err != nil {
			return err
		}
	}

	p.hash = hex.EncodeToString(digest.Sum(nil))

	return nil
}
