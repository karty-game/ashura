//go:build !js

package levelbuild

import (
	"encoding/binary"
	"math"

	bake "github.com/karty-game/karty-webgpu/pkg/lightmap/bake"
)

func indexSupportRows(report bake.SurfaceEdgeReport, receivers []bake.Receiver) (bake.ReconstructionAdjacency, error) {
	var rows bake.ReconstructionAdjacency

	edges := report.Edges
	if uint64(len(receivers)) > math.MaxUint32 || uint64(len(edges)) > math.MaxUint32 {
		return rows, ErrInvalid
	}

	owners, err := supportReportOwners(report, receivers)
	if err != nil {
		return rows, err
	}

	indices := make(map[uint64]uint32, len(receivers))

	active := make([]bool, len(receivers))
	for index, receiver := range receivers {
		indices[receiver.ID] = uint32(index)
		active[index] = owners[receiver.ID] == supportOwnerActive
	}

	rows.Owners = make([]uint32, 0, min(len(receivers), len(edges)))
	rows.Offsets = make([]uint32, 1, cap(rows.Owners)+1)
	rows.Targets = make([]uint32, len(edges))

	var previous uint64
	for index, edge := range edges {
		if index == 0 || previous != edge.From {
			from, found := indices[edge.From]
			if !found || len(rows.Owners) >= len(receivers) {
				return bake.ReconstructionAdjacency{}, ErrInvalid
			}

			if index > 0 {
				rows.Offsets = append(rows.Offsets, uint32(index))
			}

			rows.Owners = append(rows.Owners, from)
			previous = edge.From
		}
	}

	if len(edges) > 0 {
		//nolint:gosec // G115: the complete target count is bounded by MaxUint32.
		rows.Offsets = append(rows.Offsets, uint32(len(rows.Targets)))
	}

	if err = indexSupportTargets(rows.Targets, edges, indices, active); err != nil {
		return bake.ReconstructionAdjacency{}, err
	}

	return rows, nil
}

func supportPartEnd(rows bake.ReconstructionAdjacency, first int) int {
	var count uint64

	last := first
	for last < len(rows.Owners) {
		count++

		bytes := uint64(supportPartHeader) + count*supportRowBytes + uint64(
			rows.Offsets[last+1]-rows.Offsets[first],
		)*supportEdgeBytes
		if bytes > supportArtifactBytes {
			break
		}

		last++
	}

	return last
}

// Every row is whole: owner/count words followed by original-index targets.
// Unit weight and tested visibility are fixed properties of generated support.
//
//nolint:gosec // G115: nonnegative ordered row endpoints and the complete 128 MiB size are admitted before numeric narrowing.
func encodeSupportPart(rows bake.ReconstructionAdjacency, first, last int) ([]byte, error) {
	if first < 0 || first >= last || last > len(rows.Owners) || len(rows.Offsets) != len(rows.Owners)+1 {
		return nil, ErrInvalid
	}

	if rows.Offsets[first] >= rows.Offsets[last] || uint64(rows.Offsets[last]) > uint64(len(rows.Targets)) {
		return nil, ErrInvalid
	}

	targets := uint64(rows.Offsets[last]) - uint64(rows.Offsets[first])

	bytes := uint64(supportPartHeader) + uint64(last-first)*supportRowBytes + targets*supportEdgeBytes
	if targets == 0 || bytes > supportArtifactBytes {
		return nil, ErrInvalid
	}

	data := make([]byte, supportPartHeader, int(bytes))
	copy(data, supportPartMagic)
	binary.LittleEndian.PutUint64(data[8:], uint64(last-first))
	binary.LittleEndian.PutUint64(data[16:], targets)

	for row := first; row < last; row++ {
		start, end := rows.Offsets[row], rows.Offsets[row+1]
		if start >= end || uint64(end) > uint64(len(rows.Targets)) {
			return nil, ErrInvalid
		}

		data = binary.LittleEndian.AppendUint32(data, rows.Owners[row])

		data = binary.LittleEndian.AppendUint32(data, end-start)
		for _, target := range rows.Targets[start:end] {
			data = binary.LittleEndian.AppendUint32(data, target)
		}
	}

	return data, nil
}

func appendSupportPart(rows bake.ReconstructionAdjacency, data []byte) (bake.ReconstructionAdjacency, error) {
	if len(data) < supportPartHeader || len(data) > supportArtifactBytes || string(data[:8]) != supportPartMagic {
		return bake.ReconstructionAdjacency{}, ErrInvalid
	}

	count, targets := binary.LittleEndian.Uint64(data[8:]), binary.LittleEndian.Uint64(data[16:])
	if count == 0 || targets < count || count > uint64(cap(rows.Owners))-uint64(len(rows.Owners)) ||
		targets > uint64(cap(rows.Targets))-uint64(len(rows.Targets)) ||
		uint64(len(data)) != supportPartHeader+count*supportRowBytes+targets*supportEdgeBytes {
		return bake.ReconstructionAdjacency{}, ErrInvalid
	}

	remaining := targets

	position := supportPartHeader
	for range count {
		if len(data)-position < supportRowBytes {
			return bake.ReconstructionAdjacency{}, ErrInvalid
		}

		owner, count := binary.LittleEndian.Uint32(data[position:]), binary.LittleEndian.Uint32(data[position+4:])

		position += supportRowBytes
		if count == 0 || uint64(count) > remaining || uint64(count) > (uint64(len(data))-uint64(position))/supportEdgeBytes {
			return bake.ReconstructionAdjacency{}, ErrInvalid
		}

		rows.Owners = append(rows.Owners, owner)
		for range count {
			rows.Targets = append(rows.Targets, binary.LittleEndian.Uint32(data[position:]))
			position += supportEdgeBytes
		}

		remaining -= uint64(count)
		//nolint:gosec // G115: the manifest bounds the complete target count by MaxUint32.
		rows.Offsets = append(rows.Offsets, uint32(len(rows.Targets)))
	}

	if position != len(data) || remaining != 0 {
		return bake.ReconstructionAdjacency{}, ErrInvalid
	}

	return rows, nil
}
