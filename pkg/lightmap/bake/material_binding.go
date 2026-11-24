//go:build !js

package lightmapbake

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"math"

	"github.com/karty-game/ashura/internal/modeldata"
)

// reflectanceBinding survives independent chunk batching, texture-table remaps
// and the front/back receiver split. Visibility sidedness remains on the actual
// scene material; the receiver needs the same reflectance, not its batch number.
func reflectanceBinding(doc modeldata.Document, material modeldata.Material) string {
	const tintBytes = 12

	var identity [tintBytes + sha256.Size]byte // Three float32 tint channels and one SHA256 texture digest.
	for channel := range 3 {
		binary.LittleEndian.PutUint32(identity[channel*4:], math.Float32bits(canonicalTint(material.Color[channel])))
	}

	if material.Albedo >= 0 {
		digest := sha256.Sum256(doc.Textures[material.Albedo].Data)
		copy(identity[tintBytes:], digest[:])
	}

	digest := sha256.Sum256(identity[:])

	return "material:" + hex.EncodeToString(digest[:])
}

func canonicalTint(value float32) float32 {
	if value == 0 {
		return 0
	}

	return value
}
