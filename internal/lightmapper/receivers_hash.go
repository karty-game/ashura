package lightmapper

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"hash"
	"math"
)

// receiverIdentityVersion separates the exact binary field representation from
// the historical JSON identity. Existing checkpoints cannot silently alias it.
const receiverIdentityVersion = "karty.receiver-identity-binary@2\x00"
const receiverFloatExponentMask = uint32(0x7f800000)

type receiverIdentityWriter struct {
	digest  hash.Hash
	buffer  [4096]byte
	used    int
	invalid bool
}

// ReceiverHash binds all support, frame and stable-identity fields in order.
// It streams exact float32 bits without materializing a giant JSON allocation.
// Slice absence is distinct from an empty slice; nonfinite values are rejected.
func ReceiverHash(receivers []Receiver) (string, error) {
	writer := receiverIdentityWriter{digest: sha256.New()}
	_, _ = writer.digest.Write([]byte(receiverIdentityVersion))
	writer.length(len(receivers), receivers == nil)

	for _, receiver := range receivers {
		writer.receiver(receiver)
	}

	if writer.invalid {
		return "", ErrInvalid
	}

	writer.flush()

	return hex.EncodeToString(writer.digest.Sum(nil)), nil
}

func (writer *receiverIdentityWriter) flush() {
	if writer.used != 0 {
		_, _ = writer.digest.Write(writer.buffer[:writer.used])
		writer.used = 0
	}
}
func (writer *receiverIdentityWriter) word(value uint32) {
	if writer.used > len(writer.buffer)-4 {
		writer.flush()
	}

	binary.LittleEndian.PutUint32(writer.buffer[writer.used:], value)
	writer.used += 4
}
func (writer *receiverIdentityWriter) wide(value uint64) {
	writer.word(uint32(value & math.MaxUint32))
	writer.word(uint32(value >> 32))
}
func (writer *receiverIdentityWriter) length(count int, absent bool) {
	if absent {
		writer.wide(math.MaxUint64)

		return
	}

	writer.wide(uint64(count)) //nolint:gosec // All counts come from len and are nonnegative.
}
func (writer *receiverIdentityWriter) number(value float32) {
	bits := math.Float32bits(value)
	if bits&receiverFloatExponentMask == receiverFloatExponentMask {
		writer.invalid = true
	}

	writer.word(bits)
}
func (writer *receiverIdentityWriter) vector(value Vec3) {
	writer.numbers(value[:])
}

// Fixed records reserve the buffer once, rather than testing/flushing it for
// every scalar. The stream, float bits and nonfinite admission stay identical.
func (writer *receiverIdentityWriter) numbers(values []float32) {
	if writer.used > len(writer.buffer)-4*len(values) {
		writer.flush()
	}

	for _, value := range values {
		bits := math.Float32bits(value)
		if bits&receiverFloatExponentMask == receiverFloatExponentMask {
			writer.invalid = true
		}

		binary.LittleEndian.PutUint32(writer.buffer[writer.used:], bits)
		writer.used += 4
	}
}
func (writer *receiverIdentityWriter) receiver(receiver Receiver) {
	writer.wide(receiver.ID)
	writer.word(receiver.Pixel)
	writer.word(receiver.Chart)
	writer.word(uint32(receiver.Side)) //nolint:gosec // Preserve the exact signed-side bits in the identity.
	writer.length(len(receiver.Pieces), receiver.Pieces == nil)

	for _, piece := range receiver.Pieces {
		writer.word(piece.Triangle)
		writer.length(len(piece.Barycentric), piece.Barycentric == nil)

		for _, vertex := range piece.Barycentric {
			writer.vector(vertex)
		}

		writer.number(piece.Area)
	}

	writer.length(len(receiver.Patches), receiver.Patches == nil)

	for _, patch := range receiver.Patches {
		writer.patch(patch)
	}

	writer.number(receiver.Coverage)
	writer.vector(receiver.Position)
	writer.vector(receiver.GeometricNormal)
	writer.vector(receiver.Normal)
	writer.vector(receiver.Tangent)
	writer.number(receiver.Handedness)
}
func (writer *receiverIdentityWriter) patch(patch ReceiverPatch) {
	writer.numbers([]float32{
		patch.Positions[0][0], patch.Positions[0][1], patch.Positions[0][2],
		patch.Positions[1][0], patch.Positions[1][1], patch.Positions[1][2],
		patch.Positions[2][0], patch.Positions[2][1], patch.Positions[2][2],
		patch.Normals[0][0], patch.Normals[0][1], patch.Normals[0][2],
		patch.Normals[1][0], patch.Normals[1][1], patch.Normals[1][2],
		patch.Normals[2][0], patch.Normals[2][1], patch.Normals[2][2],
		patch.UV[0][0], patch.UV[0][1], patch.UV[1][0], patch.UV[1][1], patch.UV[2][0], patch.UV[2][1],
		patch.Area,
	})
	writer.word(patch.Triangle)
	writer.word(patch.Material)
	writer.numbers([]float32{
		patch.GeometricNormal[0], patch.GeometricNormal[1], patch.GeometricNormal[2],
		patch.Tangent[0], patch.Tangent[1], patch.Tangent[2],
		patch.RawTangent[0], patch.RawTangent[1], patch.RawTangent[2],
		patch.RawBitangent[0], patch.RawBitangent[1], patch.RawBitangent[2],
		patch.Handedness,
	})
}

// MaterializeReceiverPatches binds receiver geometry directly, without building
// a second visibility BVH or cloning the complete scene's texture/material data.
// The caller retains the original clipped support; visibility uses its own mesh.
func MaterializeReceiverPatches(triangles []Triangle, receivers []Receiver) error {
	patches := make([][]ReceiverPatch, len(receivers))
	for index, receiver := range receivers {
		prepared, err := receiverPatches(triangles, receiver)
		if err != nil {
			return err
		}

		patches[index] = prepared
	}

	for index := range receivers {
		receivers[index].Patches = patches[index]
	}

	return nil
}
