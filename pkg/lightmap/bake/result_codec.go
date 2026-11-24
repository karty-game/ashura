//go:build !js

//nolint:mnd // The checksummed binary record specifies fixed byte offsets, vector widths and flag bits.
package lightmapbake

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"io"
	"math"

	"github.com/karty-game/ashura/internal/lightmapper"
)

// EncodeScene stores the immutable scene independently from receiver jobs.
func EncodeScene(scene *PreparedScene) ([]byte, error) { return lightmapper.EncodeScene(scene) }

// DecodeScene admits a complete cached scene before compute resource creation.
func DecodeScene(data []byte) (*PreparedScene, error) { return lightmapper.DecodeScene(data) }

// WithLighting reuses checked geometry acceleration and changes only light preparation.
func WithLighting(scene *PreparedScene, emitters []Emitter, environment Environment) (*PreparedScene, error) {
	return lightmapper.WithLighting(scene, emitters, environment)
}

const linearRecordWords = 58

// EncodeResult stores linear means, variances and diagnostic contributions in a
// compact checksummed build artifact. It does not quantize to RGBE or GPU blocks.
//
//nolint:funlen,gosec // Write the fixed checksummed record in one audited field order.
func EncodeResult(result Result) ([]byte, error) {
	count := len(result.Estimates)
	if count == 0 || count > 1<<20 {
		return nil, ErrBounds
	}

	metadata := result
	metadata.Estimates = nil

	header, err := json.Marshal(metadata)
	if err != nil {
		return nil, err
	}

	if len(header) > 64<<10 {
		return nil, ErrBounds
	}

	bodySize := 8 + len(header) + count*linearRecordWords*4
	if bodySize > 256<<20 {
		return nil, ErrBounds
	}

	data := make([]byte, 48+bodySize)
	copy(data, "KARTYLR1")
	binary.LittleEndian.PutUint64(data[8:], uint64(bodySize))
	body := data[48:]
	binary.LittleEndian.PutUint32(body, uint32(len(header)))
	binary.LittleEndian.PutUint32(body[4:], uint32(count))
	copy(body[8:], header)
	offset := 8 + len(header)
	put := func(number float32) error {
		if math.IsNaN(float64(number)) || math.IsInf(float64(number), 0) || number < 0 {
			return ErrInvalid
		}

		binary.LittleEndian.PutUint32(body[offset:], math.Float32bits(number))
		offset += 4

		return nil
	}

	for _, estimate := range result.Estimates {
		if estimate.Count == 0 {
			return nil, ErrInvalid
		}

		binary.LittleEndian.PutUint64(body[offset:], estimate.ReceiverID)
		binary.LittleEndian.PutUint32(body[offset+8:], estimate.Count)
		offset += 12
		vectors := make([]Vec3, 0, 18)
		vectors = append(vectors, estimate.Statistics[:]...)
		vectors = append(vectors, estimate.SecondMoments[:]...)
		vectors = append(vectors, estimate.Direct, estimate.Environment, estimate.Reflected)
		vectors = append(vectors, estimate.RNM[:]...)

		vectors = append(vectors, estimate.Variance[:]...)
		for _, vector := range vectors {
			for _, number := range vector {
				if err = put(number); err != nil {
					return nil, err
				}
			}
		}

		var flags uint32
		if estimate.VarianceValid {
			flags |= 1
		}

		if estimate.SecondMomentsValid {
			flags |= 2
		}

		binary.LittleEndian.PutUint32(body[offset:], flags)
		offset += 4
	}

	digest := sha256.Sum256(body)
	copy(data[16:48], digest[:])

	return data, nil
}

// DecodeResult verifies all samples before any output is admitted for publish.
//
//nolint:funlen,cyclop,gocognit,gocyclo,gosec // Admit the complete checksummed linear result before exposing estimates.
func DecodeResult(data []byte) (Result, error) {
	var result Result
	if len(data) < 56 || len(data) > 256<<20 || string(data[:8]) != "KARTYLR1" ||
		binary.LittleEndian.Uint64(data[8:]) != uint64(len(data)-48) {
		return result, ErrInvalid
	}

	digest := sha256.Sum256(data[48:])
	if !bytes.Equal(data[16:48], digest[:]) {
		return result, ErrInvalid
	}

	body := data[48:]

	size, count := uint64(binary.LittleEndian.Uint32(body)), uint64(binary.LittleEndian.Uint32(body[4:]))
	if size > 64<<10 || count == 0 || count > 1<<20 || 8+size+count*linearRecordWords*4 != uint64(len(body)) {
		return result, ErrInvalid
	}

	decoder := json.NewDecoder(bytes.NewReader(body[8 : 8+size]))
	decoder.DisallowUnknownFields()

	if decoder.Decode(&result) != nil || decoder.Decode(new(any)) != io.EOF {
		return Result{}, ErrInvalid
	}

	if result.Schema != lightmapper.ResultSchema || result.Algorithm != lightmapper.Algorithm || len(result.SceneHash) != 64 ||
		len(result.ReceiverHash) != 64 ||
		len(result.Estimates) != 0 {
		return Result{}, ErrInvalid
	}

	result.Estimates = make([]Estimate, int(count))
	offset := 8 + int(size)
	ids := make(map[uint64]bool, int(count))
	get := func() (float32, error) {
		number := readNumber(body[offset:])
		offset += 4

		if math.IsNaN(float64(number)) || math.IsInf(float64(number), 0) || number < 0 {
			return 0, ErrInvalid
		}

		return number, nil
	}

	for i := range result.Estimates {
		estimate := &result.Estimates[i]
		estimate.ReceiverID = binary.LittleEndian.Uint64(body[offset:])
		estimate.Count = binary.LittleEndian.Uint32(body[offset+8:])
		offset += 12

		if estimate.Count == 0 || ids[estimate.ReceiverID] {
			return Result{}, ErrInvalid
		}

		ids[estimate.ReceiverID] = true

		vectors := make([]*Vec3, 0, 18)
		for component := range 4 {
			vectors = append(vectors, &estimate.Statistics[component])
		}

		for component := range 4 {
			vectors = append(vectors, &estimate.SecondMoments[component])
		}

		vectors = append(vectors, &estimate.Direct, &estimate.Environment, &estimate.Reflected)
		for component := range 3 {
			vectors = append(vectors, &estimate.RNM[component])
		}

		for component := range 4 {
			vectors = append(vectors, &estimate.Variance[component])
		}

		for _, vector := range vectors {
			for component := range 3 {
				number, err := get()
				if err != nil {
					return Result{}, err
				}

				vector[component] = number
			}
		}

		flags := binary.LittleEndian.Uint32(body[offset:])
		offset += 4

		if flags & ^uint32(3) != 0 {
			return Result{}, ErrInvalid
		}

		estimate.VarianceValid = flags&1 != 0
		estimate.SecondMomentsValid = flags&2 != 0
	}

	return result, nil
}
