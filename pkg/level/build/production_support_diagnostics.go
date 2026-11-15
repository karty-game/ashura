//go:build !js

//nolint:mnd // The diagnostic wire stores uint64 IDs/counts and twelve float32 geometry channels per contact.
package levelbuild

import (
	"encoding/binary"
	"math"

	bake "github.com/karty-game/karty-webgpu/pkg/lightmap/bake"
)

const (
	supportDiagnosticMagic  = "KRTYSD01"
	supportDiagnosticHeader = 48
	supportContactBytes     = 52
)

const (
	supportOwnerActive uint8 = iota + 1
	supportOwnerIsolated
)

//nolint:gosec // G115: The complete 128 MiB wire-size preflight bounds each contact ID count before narrowing.
func encodeSupportDiagnostics(report bake.SurfaceEdgeReport) ([]byte, error) {
	size := uint64(supportDiagnosticHeader) + uint64(len(report.IsolatedReceiverIDs))*8
	for _, contact := range report.AmbiguousContacts {
		size += supportContactBytes + uint64(len(contact.ReceiverIDs))*8
		if size > supportArtifactBytes {
			return nil, ErrInvalid
		}
	}

	if size > supportArtifactBytes {
		return nil, ErrInvalid
	}

	data := make([]byte, supportDiagnosticHeader, int(size))
	copy(data, supportDiagnosticMagic)
	binary.LittleEndian.PutUint64(data[8:], uint64(len(report.IsolatedReceiverIDs)))
	binary.LittleEndian.PutUint64(data[16:], uint64(len(report.AmbiguousContacts)))
	binary.LittleEndian.PutUint64(data[24:], report.SegmentCount)
	binary.LittleEndian.PutUint64(data[32:], report.CandidateWork)
	binary.LittleEndian.PutUint64(data[40:], report.ExpansionWork)

	for _, id := range report.IsolatedReceiverIDs {
		data = binary.LittleEndian.AppendUint64(data, id)
	}

	for _, contact := range report.AmbiguousContacts {
		for _, vector := range [4]bake.Vec3{contact.Start, contact.End, contact.Normals[0], contact.Normals[1]} {
			for _, number := range vector {
				data = binary.LittleEndian.AppendUint32(data, math.Float32bits(number))
			}
		}

		data = binary.LittleEndian.AppendUint32(data, uint32(len(contact.ReceiverIDs)))
		for _, id := range contact.ReceiverIDs {
			data = binary.LittleEndian.AppendUint64(data, id)
		}
	}

	return data, nil
}

//nolint:gosec // G115: The complete wire and decoded-size preflight bounds every allocation and cursor conversion.
func decodeSupportDiagnostics(data []byte, maximumReceivers uint64) (bake.SurfaceEdgeReport, error) {
	var report bake.SurfaceEdgeReport

	isolated, contacts, err := preflightSupportDiagnostics(data, maximumReceivers)
	if err != nil {
		return report, err
	}

	report.SegmentCount = binary.LittleEndian.Uint64(data[24:])
	report.CandidateWork = binary.LittleEndian.Uint64(data[32:])
	report.ExpansionWork = binary.LittleEndian.Uint64(data[40:])
	report.IsolatedReceiverIDs = make([]uint64, int(isolated))
	report.AmbiguousContacts = make([]bake.SurfaceContactDiagnostic, int(contacts))

	offset := supportDiagnosticHeader
	for index := range report.IsolatedReceiverIDs {
		report.IsolatedReceiverIDs[index] = binary.LittleEndian.Uint64(data[offset:])
		offset += 8
	}

	for index := range report.AmbiguousContacts {
		contact := &report.AmbiguousContacts[index]
		for _, vector := range []*bake.Vec3{&contact.Start, &contact.End, &contact.Normals[0], &contact.Normals[1]} {
			for channel := range 3 {
				(*vector)[channel] = math.Float32frombits(binary.LittleEndian.Uint32(data[offset:]))
				offset += 4
			}
		}

		count := int(binary.LittleEndian.Uint32(data[offset:]))
		offset += 4

		contact.ReceiverIDs = make([]uint64, count)
		for owner := range contact.ReceiverIDs {
			contact.ReceiverIDs[owner] = binary.LittleEndian.Uint64(data[offset:])
			offset += 8
		}
	}

	return report, nil
}

// preflightSupportDiagnostics checks every variable-length record before any
// decoded arrays are allocated. Its decoded-memory ceiling is twice the wire cap.
//
//nolint:gosec // G115: Length checks precede each subtraction and cap every count/cursor below the 128 MiB wire bound.
func preflightSupportDiagnostics(data []byte, maximumReceivers uint64) (uint64, uint64, error) {
	if len(data) < supportDiagnosticHeader || len(data) > supportArtifactBytes || string(data[:8]) != supportDiagnosticMagic {
		return 0, 0, ErrInvalid
	}

	isolated, contacts := binary.LittleEndian.Uint64(data[8:]), binary.LittleEndian.Uint64(data[16:])
	if isolated > maximumReceivers || isolated > uint64(len(data)-supportDiagnosticHeader)/8 {
		return 0, 0, ErrInvalid
	}

	offset := supportDiagnosticHeader + int(isolated)*8
	if contacts > uint64(len(data)-offset)/(supportContactBytes+16) {
		return 0, 0, ErrInvalid
	}

	decoded := isolated*8 + contacts*80
	for range contacts {
		if len(data)-offset < supportContactBytes {
			return 0, 0, ErrInvalid
		}

		count := uint64(binary.LittleEndian.Uint32(data[offset+48:]))

		offset += supportContactBytes
		if count < 2 || count > maximumReceivers || count > uint64(len(data)-offset)/8 {
			return 0, 0, ErrInvalid
		}

		decoded += count * 8
		if decoded > 2*supportArtifactBytes {
			return 0, 0, ErrInvalid
		}

		offset += int(count) * 8
	}

	if offset != len(data) {
		return 0, 0, ErrInvalid
	}

	return isolated, contacts, nil
}

func validateSupportReport(report bake.SurfaceEdgeReport, receivers []bake.Receiver) error {
	owners, err := supportReportOwners(report, receivers)
	if err != nil {
		return err
	}

	for index, edge := range report.Edges {
		if index > 0 && !supportEdgeBefore(report.Edges[index-1], edge) {
			return ErrInvalid
		}

		if !validSupportEdge(edge) || !supportEdgeOwners(owners, edge, index == 0 || edge.From != report.Edges[index-1].From) {
			return ErrInvalid
		}
	}

	return nil
}

// supportReportOwners admits the complete isolation diagnostics before any edge
// is consumed. Artifact decoding and in-memory reports share the same rules.
func supportReportOwners(report bake.SurfaceEdgeReport, receivers []bake.Receiver) (map[uint64]uint8, error) {
	owners := make(map[uint64]uint8, len(receivers))
	for _, receiver := range receivers {
		if owners[receiver.ID] != 0 {
			return nil, ErrInvalid
		}

		owners[receiver.ID] = supportOwnerActive
	}

	isolated := make(map[uint64]bool, len(report.IsolatedReceiverIDs))
	for index, identity := range report.IsolatedReceiverIDs {
		if owners[identity] == 0 || (index > 0 && identity <= report.IsolatedReceiverIDs[index-1]) {
			return nil, ErrInvalid
		}

		isolated[identity] = true
		owners[identity] = supportOwnerIsolated
	}

	if err := validateSupportContacts(report.AmbiguousContacts, isolated); err != nil {
		return nil, err
	}

	return owners, nil
}

func supportEdgeOwners(owners map[uint64]uint8, edge bake.ReconstructionEdge, first bool) bool {
	// Canonical source groups share their already admitted membership state.
	return (!first || owners[edge.From] == supportOwnerActive) && owners[edge.To] == supportOwnerActive
}

func validateSupportContacts(contacts []bake.SurfaceContactDiagnostic, isolated map[uint64]bool) error {
	diagnosed := make(map[uint64]bool, len(isolated))
	for _, contact := range contacts {
		if len(contact.ReceiverIDs) < 2 {
			return ErrInvalid
		}

		for index, id := range contact.ReceiverIDs {
			if !isolated[id] || (index > 0 && id <= contact.ReceiverIDs[index-1]) {
				return ErrInvalid
			}

			diagnosed[id] = true
		}

		for _, vector := range [4]bake.Vec3{contact.Start, contact.End, contact.Normals[0], contact.Normals[1]} {
			for _, number := range vector {
				if math.IsNaN(float64(number)) || math.IsInf(float64(number), 0) {
					return ErrInvalid
				}
			}
		}
	}

	if len(diagnosed) != len(isolated) {
		return ErrInvalid
	}

	return nil
}
