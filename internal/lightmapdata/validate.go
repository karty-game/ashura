package lightmapdata

import (
	"encoding/binary"
	"math"
)

// Validate bounds complete arrays and proves a finite, depth-bounded BVH.
func Validate(document *Document) error {
	if err := validateDimensions(document); err != nil {
		return err
	}

	if err := validateUV(document); err != nil {
		return err
	}

	if encodedSize(document) > MaxBytes {
		return ErrFormat
	}

	if !finiteArray(document.Triangles) {
		return ErrFormat
	}

	if err := validateReceivers(document); err != nil {
		return err
	}

	if err := validatePadding(document); err != nil {
		return err
	}

	if err := validateRNM(document); err != nil {
		return err
	}

	if err := validateTransport(document); err != nil {
		return err
	}

	return validateTree(document)
}

func validateDimensions(document *Document) error {
	if document == nil || document.Width == 0 || document.Height == 0 || document.Width > MaxDimension || document.Height > MaxDimension ||
		len(document.UVs) == 0 ||
		len(document.UVs) > MaxParts {
		return ErrFormat
	}

	pixels := int(document.Width) * int(document.Height)
	if len(document.Receivers) != pixels*ReceiverStride || len(document.Padding) != pixels*4 {
		return ErrFormat
	}

	if len(document.Nodes) == 0 || len(document.Nodes)%NodeStride != 0 || len(document.Nodes)/NodeStride > MaxNodes {
		return ErrFormat
	}

	if len(document.Triangles) == 0 || len(document.Triangles)%TriangleStride != 0 ||
		len(document.Triangles)/TriangleStride > MaxTriangles {
		return ErrFormat
	}

	return nil
}

func encodedSize(document *Document) int {
	size := headerSize + UVStride/2*len(document.UVs)
	if document.RNM() {
		size = rnmHeaderSize + UVStride/2*len(document.UVs)
	}

	if document.Transport() {
		size = transportHeaderSize + UVStride/2*len(document.UVs)
	}

	for _, field := range document.fields() {
		size += len(*field)
	}

	for _, coordinates := range document.UVs {
		size += len(coordinates)
	}

	return size
}

func validateUV(document *Document) error {
	vertices := 0

	for _, coordinates := range document.UVs {
		if len(coordinates) == 0 || len(coordinates)%UVStride != 0 || len(coordinates) > MaxVertices*UVStride {
			return ErrFormat
		}

		vertices += len(coordinates) / UVStride
		if vertices > MaxVertices {
			return ErrFormat
		}

		for offset := 0; offset < len(coordinates); offset += 4 {
			value := number(coordinates[offset:])
			if !finite(value) || value < 0 || value > 1 {
				return ErrFormat
			}
		}
	}

	return nil
}

func finiteArray(data []byte) bool {
	for offset := 0; offset < len(data); offset += 4 {
		if !finite(number(data[offset:])) {
			return false
		}
	}

	return true
}

func validateReceivers(document *Document) error {
	for offset := 0; offset < len(document.Receivers); offset += ReceiverStride {
		receiver := document.Receivers[offset : offset+ReceiverStride]
		if !finiteArray(receiver) {
			return ErrFormat
		}

		coverage := number(receiver[12:])
		if coverage != 0 && coverage != 1 && coverage != 2 {
			return ErrFormat
		}

		if coverage == 0 {
			continue
		}

		length := number(
			receiver[16:],
		)*number(
			receiver[16:],
		) + number(
			receiver[20:],
		)*number(
			receiver[20:],
		) + number(
			receiver[24:],
		)*number(
			receiver[24:],
		)
		if length < .5 || length > 1.5 || number(receiver[28:]) < 1 {
			return ErrFormat
		}
	}

	return nil
}

func validatePadding(document *Document) error {
	pixels := document.Width * document.Height
	for offset := 0; offset < len(document.Padding); offset += 4 {
		source := binary.LittleEndian.Uint32(document.Padding[offset:])
		if source == math.MaxUint32 {
			continue
		}

		if source >= pixels || number(document.Receivers[int(source)*ReceiverStride+12:]) == 0 {
			return ErrFormat
		}

		if number(document.Receivers[offset/4*ReceiverStride+12:]) > 0 && source != uint32(offset/4) {
			return ErrFormat
		}
	}

	return nil
}

type treeVisit struct{ node, depth uint32 }

func validateTree(document *Document) error {
	stack := []treeVisit{{0, 0}}
	seen := make([]bool, len(document.Nodes)/NodeStride)

	triangles := make([]bool, len(document.Triangles)/TriangleStride)
	for len(stack) > 0 {
		visit := stack[len(stack)-1]

		stack = stack[:len(stack)-1]
		if int(visit.node) >= len(seen) || visit.depth >= 63 || seen[visit.node] {
			return ErrFormat
		}

		seen[visit.node] = true

		node := document.Nodes[int(visit.node)*NodeStride:]
		if !validNodeBounds(node) {
			return ErrFormat
		}

		first, count := binary.LittleEndian.Uint32(node[12:]), binary.LittleEndian.Uint32(node[28:])
		if count == 0 {
			if first <= visit.node || uint64(first)+1 >= uint64(len(seen)) {
				return ErrFormat
			}

			stack = append(stack, treeVisit{first, visit.depth + 1}, treeVisit{first + 1, visit.depth + 1})

			continue
		}

		if !markLeaf(first, count, triangles) {
			return ErrFormat
		}
	}

	for _, visited := range seen {
		if !visited {
			return ErrFormat
		}
	}

	for _, visited := range triangles {
		if !visited {
			return ErrFormat
		}
	}

	return nil
}

func validNodeBounds(node []byte) bool {
	for axis := range 3 {
		lo, hi := number(node[axis*4:]), number(node[16+axis*4:])
		if !finite(lo) || !finite(hi) || lo > hi {
			return false
		}
	}

	return true
}

func markLeaf(first, count uint32, triangles []bool) bool {
	if uint64(first)+uint64(count) > uint64(len(triangles)) {
		return false
	}

	for index := first; index < first+count; index++ {
		if triangles[index] {
			return false
		}

		triangles[index] = true
	}

	return true
}

func number(data []byte) float32 { return math.Float32frombits(binary.LittleEndian.Uint32(data)) }
func finite(value float32) bool  { return !math.IsNaN(float64(value)) && !math.IsInf(float64(value), 0) }

func validateRNM(document *Document) error {
	if len(document.Frames) == 0 && len(document.Surfaces) == 0 {
		return nil
	}

	if len(document.Frames) != int(document.Width)*int(document.Height)*FrameStride ||
		len(document.Surfaces) != len(document.Triangles)/TriangleStride*SurfaceStride {
		return ErrFormat
	}

	for offset := 0; offset < len(document.Frames); offset += FrameStride {
		if err := validateFrame(
			document.Frames[offset:offset+FrameStride],
			document.Receivers[offset/FrameStride*ReceiverStride:],
		); err != nil {
			return err
		}
	}

	for offset := 0; offset < len(document.Surfaces); offset += SurfaceStride {
		surface := document.Surfaces[offset : offset+SurfaceStride]
		if !finiteArray(surface) {
			return ErrFormat
		}

		for axis := range 3 {
			value := number(surface[axis*4:])
			if value < 0 || value > .95 {
				return ErrFormat
			}
		}

		if number(surface[12:]) != 0 && number(surface[12:]) != 1 {
			return ErrFormat
		}
	}

	return nil
}

func validateFrame(frame, receiver []byte) error {
	if !finiteArray(frame) {
		return ErrFormat
	}

	if number(receiver[12:]) == 0 {
		for component := 0; component < FrameStride; component += 4 {
			if number(frame[component:]) != 0 {
				return ErrFormat
			}
		}

		return nil
	}

	var length, dot float32

	for axis := range 3 {
		tangent := number(frame[axis*4:])
		length += tangent * tangent
		dot += tangent * number(receiver[16+axis*4:])
	}

	const tangentTolerance = .002
	if math.Abs(float64(length-1)) > tangentTolerance || math.Abs(float64(dot)) > tangentTolerance ||
		(number(frame[12:]) != -1 && number(frame[12:]) != 1) {
		return ErrFormat
	}

	return nil
}
