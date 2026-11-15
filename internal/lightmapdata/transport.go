// Validation proves all geometry-precomputed diffuse references.
package lightmapdata

import (
	"encoding/binary"
	"math"
)

func validateTransport(document *Document) error {
	if !document.Transport() {
		if len(document.GIReceivers) != 0 || len(document.GIPatches) != 0 || len(document.GITransfers) != 0 || document.GISamples != 0 {
			return ErrFormat
		}

		return nil
	}

	if !validTransportDimensions(document) {
		return ErrFormat
	}

	size := 0
	for _, field := range document.fields() {
		size += len(*field)
	}

	if size > MaxTransportBytes {
		return ErrFormat
	}

	compact := uint32(0)

	for offset := 0; offset < len(document.ReceiverGIIndex); offset += 4 {
		index := binary.LittleEndian.Uint32(document.ReceiverGIIndex[offset:])

		covered := number(document.Receivers[offset/4*ReceiverStride+12:]) > 0
		if covered {
			if index != compact {
				return ErrFormat
			}

			compact++
		} else if index != math.MaxUint32 {
			return ErrFormat
		}
	}

	if int(compact) != len(document.GIReceivers)/GIReceiverStride {
		return ErrFormat
	}

	if err := validatePatches(document.GIPatches); err != nil {
		return err
	}

	return validateTransfers(document)
}

func validatePatches(patches []byte) error {
	for offset := 0; offset < len(patches); offset += GIPatchStride {
		patch := patches[offset : offset+GIPatchStride]
		if !finiteArray(patch) {
			return ErrFormat
		}

		var squared float32

		for axis := range 3 {
			normal := number(patch[16+axis*4:])
			squared += normal * normal

			reflectance := number(patch[32+axis*4:])
			if reflectance < 0 || reflectance > .95 {
				return ErrFormat
			}
		}

		const normalTolerance = .002
		if math.Abs(float64(squared-1)) > normalTolerance {
			return ErrFormat
		}

		sky, ground := number(patch[12:]), number(patch[28:])
		if sky < 0 || ground < 0 || sky+ground > 1.002 || (number(patch[44:]) != 0 && number(patch[44:]) != 1) {
			return ErrFormat
		}
	}

	return nil
}

func validateTransfers(document *Document) error {
	cursor := uint32(0)

	for offset := 0; offset < len(document.GIReceivers); offset += GIReceiverStride {
		receiver := document.GIReceivers[offset : offset+GIReceiverStride]

		start, count := binary.LittleEndian.Uint32(receiver), binary.LittleEndian.Uint32(receiver[4:])
		if start != cursor || count > MaxGITransfers || uint64(start)+uint64(count) > uint64(len(document.GITransfers)/GITransferStride) {
			return ErrFormat
		}

		totals := [3]float32{}

		for axis := range 3 {
			sky, ground := number(receiver[8+axis*4:]), number(receiver[20+axis*4:])
			if !finite(sky) || !finite(ground) || sky < 0 || ground < 0 {
				return ErrFormat
			}

			totals[axis] = sky + ground
		}

		if err := validateReceiverLinks(document, start, count, &totals); err != nil {
			return err
		}

		for _, total := range totals {
			if !finite(total) || total > 2.002 {
				return ErrFormat
			}
		}

		cursor += count
	}

	if int(cursor) != len(document.GITransfers)/GITransferStride {
		return ErrFormat
	}

	return nil
}

func validTransportDimensions(document *Document) bool {
	if !document.RNM() || document.GISamples < 16 || document.GISamples > 128 ||
		len(document.ReceiverGIIndex) != int(document.Width)*int(document.Height)*4 ||
		len(document.GIReceivers) == 0 ||
		len(document.GIReceivers)%GIReceiverStride != 0 ||
		len(document.GIPatches)%GIPatchStride != 0 ||
		len(document.GITransfers)%GITransferStride != 0 {
		return false
	}

	return true
}

func validateReceiverLinks(document *Document, start, count uint32, totals *[3]float32) error {
	var patches [MaxGITransfers]uint32

	for link := range count {
		transfer := document.GITransfers[int(start+link)*GITransferStride:]

		patch := binary.LittleEndian.Uint32(transfer)
		if uint64(patch) >= uint64(len(document.GIPatches)/GIPatchStride) {
			return ErrFormat
		}

		for previous := range link {
			if patches[previous] == patch {
				return ErrFormat
			}
		}

		patches[link] = patch

		for axis := range 3 {
			weight := number(transfer[4+axis*4:])
			if !finite(weight) || weight < 0 {
				return ErrFormat
			}

			totals[axis] += weight
		}
	}

	return nil
}
