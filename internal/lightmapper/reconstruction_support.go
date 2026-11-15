//go:build !js

package lightmapper

import "math"

// ReconstructionAdjacency names positions in the exact ordered receiver set.
// Every row contains visible unit-weight support in canonical receiver-ID order.
type ReconstructionAdjacency struct {
	Owners, Offsets, Targets []uint32
	Isolated                 []uint32
}

// ReconstructionSupport privately owns admitted static adjacency. Lighting,
// estimates and reconstruction strength do not participate in this geometry.
type ReconstructionSupport struct {
	identities      []uint64
	guides          []reconstructionGuide
	retained        []uint32
	original        []int
	offsets, starts []uint32
	targets         []uint32
}

// PrepareReconstructionSupport admits every original identity and edge before
// retaining dense CSR links. It retains no caller-owned mutable arrays.
// Zero tangent frames retain their estimates and cannot enter a filter halo.
func PrepareReconstructionSupport(receivers []Receiver, adjacency ReconstructionAdjacency) (*ReconstructionSupport, error) {
	if len(receivers) == 0 || uint64(len(receivers)) > math.MaxUint32 || uint64(len(adjacency.Targets)) > math.MaxUint32 ||
		len(adjacency.Offsets) != len(adjacency.Owners)+1 || adjacency.Offsets[0] != 0 ||
		uint64(adjacency.Offsets[len(adjacency.Owners)]) != uint64(len(adjacency.Targets)) {
		return nil, errReconstruction
	}

	support := &ReconstructionSupport{
		identities: make([]uint64, len(receivers)),
		guides:     make([]reconstructionGuide, len(receivers)),
		retained:   make([]uint32, len(receivers)),
		original:   make([]int, 0, len(receivers)),
		targets:    make([]uint32, 0, len(adjacency.Targets)),
	}

	identities := make(map[uint64]bool, len(receivers))
	for index, receiver := range receivers {
		if identities[receiver.ID] {
			return nil, errReconstruction
		}

		identities[receiver.ID] = true
		support.identities[index] = receiver.ID
		support.guides[index] = reconstructionReceiverGuide(receiver)

		support.retained[index] = omittedReconstructionFrame
		if receiver.Tangent != (Vec3{}) {
			//nolint:gosec // G115: the complete original count is bounded above by MaxUint32.
			support.retained[index] = uint32(len(support.original))
			support.original = append(support.original, index)
		}
	}

	support.offsets = make([]uint32, len(support.original)+1)
	support.starts = make([]uint32, len(support.original))

	if err := support.admitRows(receivers, adjacency); err != nil {
		return nil, err
	}

	return support, nil
}

func (support *ReconstructionSupport) admitRows(receivers []Receiver, adjacency ReconstructionAdjacency) error {
	isolated, err := reconstructionIsolation(len(receivers), adjacency.Isolated)
	if err != nil {
		return err
	}

	for row, owner := range adjacency.Owners {
		first, last := adjacency.Offsets[row], adjacency.Offsets[row+1]
		if uint64(owner) >= uint64(len(receivers)) || first >= last || uint64(last) > uint64(len(adjacency.Targets)) || isolated[owner] ||
			row > 0 && receivers[adjacency.Owners[row-1]].ID >= receivers[owner].ID {
			return errReconstruction
		}

		from := support.retained[owner]

		for index := first; index < last; index++ {
			target := adjacency.Targets[index]
			if uint64(target) >= uint64(len(receivers)) || target == owner || isolated[target] ||
				index > first && receivers[adjacency.Targets[index-1]].ID >= receivers[target].ID {
				return errReconstruction
			}

			destination := support.retained[target]
			if from == omittedReconstructionFrame || destination == omittedReconstructionFrame {
				continue
			}

			if support.offsets[from+1] == 0 {
				//nolint:gosec // G115: PrepareReconstructionSupport bounds the complete target count by MaxUint32.
				support.starts[from] = uint32(len(support.targets))
			}

			support.offsets[from+1]++
			support.targets = append(support.targets, destination)
		}
	}

	for index := 1; index < len(support.offsets); index++ {
		support.offsets[index] += support.offsets[index-1]
	}

	return nil
}

func reconstructionIsolation(count int, indices []uint32) ([]bool, error) {
	isolated := make([]bool, count)
	for _, index := range indices {
		if uint64(index) >= uint64(len(isolated)) || isolated[index] {
			return nil, errReconstruction
		}

		isolated[index] = true
	}

	return isolated, nil
}
