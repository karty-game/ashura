package lightmapper

import "fmt"

type receiverRowBuilder struct {
	triangles     []Triangle
	coordinates   [][3]Vec2
	width, height uint32
	pixels        receiverPixelIndex
	receivers     []Receiver
}

//nolint:gosec,mnd // Every receiver owns a distinct admitted pixel; the index+1 sentinel fits the uint32 pixel domain. Coverage tolerance admits float32 accumulation.
func (state *receiverRowBuilder) appendPiece(index int, piece receiverPixelPiece, frame receiverPatchFrame, arena *[]ReceiverPatch) error {
	slot := state.pixels.get(piece.pixel)

	fresh := slot == 0
	if fresh {
		state.receivers = append(state.receivers, Receiver{ID: uint64(piece.pixel) + 1, Pixel: piece.pixel, Side: 1})
		slot = uint32(len(state.receivers))
		state.pixels.set(piece.pixel, slot)
	}

	receiver := &state.receivers[slot-1]
	if err := state.validateOverlap(receiver, piece, index); err != nil {
		return err
	}

	patches := receiver.Patches

	start := 0
	if fresh {
		patches, start = *arena, len(*arena)
	}

	patches, err := appendReceiverClippedPatches(patches, state.triangles[index], frame, piece)
	if err != nil {
		return fmt.Errorf("pixel %d triangle %d precise patches: %w", piece.pixel, index, err)
	}

	if fresh {
		*arena = patches
		// Exact-capacity spans make later appends copy instead of extending into a neighbor.
		receiver.Patches = patches[start:len(patches):len(patches)]
	} else {
		receiver.Patches = patches
	}

	receiver.Pieces = append(receiver.Pieces, piece.piece)

	receiver.Coverage += piece.piece.Area
	if receiver.Coverage > 1.0001 {
		return fmt.Errorf("pixel %d triangle %d: coverage %g: %w", piece.pixel, index, receiver.Coverage, errReceiver)
	}

	return nil
}

func (state *receiverRowBuilder) validateOverlap(receiver *Receiver, piece receiverPixelPiece, index int) error {
	for _, existing := range receiver.Pieces {
		if receiverPositiveUVOverlap(piece.polygon, state.coordinates[existing.Triangle], state.width, state.height) {
			return fmt.Errorf(
				"pixel %d: positive UV overlap between triangles %d (id %d, UV %v) and %d (id %d, UV %v): %w",
				piece.pixel,
				existing.Triangle,
				state.triangles[existing.Triangle].ID,
				state.coordinates[existing.Triangle],
				index,
				state.triangles[index].ID,
				state.coordinates[index],
				errReceiver,
			)
		}
	}

	return nil
}
