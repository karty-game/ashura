// Package resourcedata retains an immutable, integrity-checked resource payload.
// Only the resource reader may create one after its existing size/SHA admission.
package resourcedata

// Payload has no mutable public byte accessor. A zero value is not verified.
type Payload struct {
	data   []byte
	digest [32]byte
	valid  bool
}

// OwnVerified retains the reader's private bytes without hashing or copying them.
// The reader must have verified this exact digest and relinquishes mutation.
func OwnVerified(data []byte, digest [32]byte) *Payload {
	return &Payload{data: data, digest: digest, valid: true}
}

// Bytes is an internal immutable view, never a caller-owned mutable buffer.
func Bytes(payload *Payload) []byte {
	if !Valid(payload) {
		return nil
	}

	return payload.data
}

func Valid(payload *Payload) bool { return payload != nil && payload.valid }

// Digest returns the checksum established by the reader's integrity admission.
func Digest(payload *Payload) [32]byte {
	if !Valid(payload) {
		return [32]byte{}
	}

	return payload.digest
}
