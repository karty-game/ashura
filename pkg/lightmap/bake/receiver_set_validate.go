//go:build !js

package lightmapbake

import (
	"math"
	"slices"
)

//nolint:gosec // G115: dimensions are capped at 2048 before converting the pixel allocation size.
func validReceiverSet(set ReceiverSet) error {
	pixels := uint64(set.Width) * uint64(set.Height)
	if set.Schema != receiverSetSchema || set.Width == 0 || set.Height == 0 || set.Width > 2048 || set.Height > 2048 ||
		uint64(len(set.Gutters)) != pixels || len(set.Receivers) == 0 || uint64(len(set.Receivers)) > pixels {
		return ErrInvalid
	}

	covered := make([]bool, int(pixels))

	ids := make([]uint64, 0, len(set.Receivers))
	for _, receiver := range set.Receivers {
		if uint64(receiver.Pixel) >= pixels || covered[receiver.Pixel] {
			return ErrInvalid
		}

		if err := validReceiverRecord(receiver); err != nil {
			return err
		}

		covered[receiver.Pixel] = true
		ids = append(ids, receiver.ID)
	}

	slices.Sort(ids)

	for i := 1; i < len(ids); i++ {
		if ids[i] == ids[i-1] {
			return ErrInvalid
		}
	}

	for _, owner := range set.Gutters {
		if owner != math.MaxUint32 && (uint64(owner) >= pixels || !covered[owner]) {
			return ErrInvalid
		}
	}

	return nil
}

func validReceiverRecord(receiver Receiver) error {
	if len(receiver.Patches) == 0 || receiver.Side != 1 && receiver.Side != -1 || !receiverSetPositive(receiver.Coverage) {
		return ErrInvalid
	}

	if uint64(len(receiver.Patches)) > math.MaxUint32 || uint64(len(receiver.Pieces)) > math.MaxUint32 {
		return ErrBounds
	}

	for _, value := range []Vec3{receiver.Position, receiver.GeometricNormal, receiver.Normal, receiver.Tangent} {
		if !receiverSetVector(value) {
			return ErrInvalid
		}
	}

	if !receiverSetFinite(receiver.Handedness) {
		return ErrInvalid
	}

	for _, piece := range receiver.Pieces {
		if !validReceiverPiece(piece) {
			return ErrInvalid
		}
	}

	var coverage float64

	for _, patch := range receiver.Patches {
		if !validReceiverPatch(patch) {
			return ErrInvalid
		}

		coverage += float64(patch.Area)
	}

	if math.Abs(coverage-float64(receiver.Coverage)) > 1e-4*coverage {
		return ErrInvalid
	}

	return nil
}

func validReceiverPiece(piece FootprintPiece) bool {
	if !receiverSetPositive(piece.Area) || len(piece.Barycentric) < 3 || len(piece.Barycentric) > 16 {
		return false
	}

	for _, bary := range piece.Barycentric {
		if !receiverSetVector(bary) || bary[0] < -.00001 || bary[1] < -.00001 || bary[2] < -.00001 ||
			math.Abs(float64(bary[0]+bary[1]+bary[2]-1)) > 1e-4 {
			return false
		}
	}

	return true
}

func validReceiverPatch(patch ReceiverPatch) bool {
	if !receiverSetPositive(patch.Area) || !receiverSetFinite(patch.Handedness) {
		return false
	}

	for _, value := range patch.Positions {
		if !receiverSetVector(value) {
			return false
		}
	}

	for _, value := range patch.Normals {
		if !receiverSetVector(value) {
			return false
		}
	}

	for _, value := range patch.UV {
		if !receiverSetFinite(value[0]) || !receiverSetFinite(value[1]) {
			return false
		}
	}

	for _, value := range []Vec3{patch.GeometricNormal, patch.Tangent, patch.RawTangent, patch.RawBitangent} {
		if !receiverSetVector(value) {
			return false
		}
	}

	return true
}
