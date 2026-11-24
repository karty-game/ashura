package modeldata

import "github.com/karty-game/ashura/internal/resourcedata"

// VerifiedDocument binds parsed model metadata to immutable, checksum-verified bytes.
type VerifiedDocument struct {
	document Document
	digest   [32]byte
}

// DecodeVerified parses the container once without copying its geometry or textures.
func DecodeVerified(payload *resourcedata.Payload) (*VerifiedDocument, error) {
	document, err := Decode(resourcedata.Bytes(payload))
	if err != nil {
		return nil, err
	}

	return &VerifiedDocument{document: document, digest: resourcedata.Digest(payload)}, nil
}

// Document returns read-only metadata and immutable buffer views.
func (verified *VerifiedDocument) Document() Document { return verified.document }

// Digest returns the checksum of the exact parsed payload.
func (verified *VerifiedDocument) Digest() [32]byte { return verified.digest }
