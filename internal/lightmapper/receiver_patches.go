package lightmapper

import "math"

//nolint:mnd // A polygon with n vertices admits at most n-2 fan triangles.
func receiverTrianglePatchArena(pieces []receiverPixelPiece, pixels receiverPixelIndex) ([]ReceiverPatch, int, error) {
	capacity := 0
	fresh := 0

	for _, piece := range pieces {
		if pixels.get(piece.pixel) != 0 {
			continue
		}

		fans := len(piece.polygon) - 2
		if fans < 1 || fans > math.MaxInt-capacity {
			return nil, 0, errReceiver
		}

		capacity += fans
		fresh++
	}

	return make([]ReceiverPatch, 0, capacity), fresh, nil
}

// receiverClippedPatches keeps fan weights in the original float64 clipped UV
// measure. Quantized barycentric provenance may become collinear for a sliver;
// it must never be used to reconstruct or reject this positive covered support.
func receiverClippedPatches(triangle Triangle, piece receiverPixelPiece) ([]ReceiverPatch, error) {
	return appendReceiverClippedPatches(nil, triangle, receiverTrianglePatchFrame(triangle), piece)
}

type receiverPatchFrame struct {
	geometric, tangent, rawTangent, rawBitangent Vec3
	handedness                                   float32
}

func receiverTrianglePatchFrame(triangle Triangle) receiverPatchFrame {
	frame := receiverPatchSample(
		ReceiverPatch{Positions: triangle.Positions, Normals: triangle.Normals, UV: triangle.UV},
		Vec3{1.0 / 3, 1.0 / 3, 1.0 / 3},
		1,
	)
	rawTangent, rawBitangent := receiverRawFrame(triangle.Positions, triangle.UV)

	return receiverPatchFrame{frame.GeometricNormal, frame.Tangent, rawTangent, rawBitangent, frame.Handedness}
}

func appendReceiverClippedPatches(
	patches []ReceiverPatch,
	triangle Triangle,
	frame receiverPatchFrame,
	piece receiverPixelPiece,
) ([]ReceiverPatch, error) {
	polygon := piece.polygon
	total := receiverPolygonArea(polygon)

	for index := 1; index+1 < len(polygon); index++ {
		corners := [3]receiverClipVertex{polygon[0], polygon[index], polygon[index+1]}

		area := receiverPolygonArea(corners[:])
		if area == 0 {
			continue
		}

		patch := ReceiverPatch{
			Area:            piece.piece.Area * float32(area/total),
			Triangle:        piece.piece.Triangle,
			Material:        triangle.Material,
			GeometricNormal: frame.geometric,
			Tangent:         frame.tangent,
			Handedness:      frame.handedness,
			RawTangent:      frame.rawTangent,
			RawBitangent:    frame.rawBitangent,
		}
		if patch.Area <= 0 {
			return nil, errReceiver
		}

		for corner, vertex := range corners {
			patch.Positions[corner] = receiverInterpolatePrecise(triangle.Positions, vertex.bary)
			patch.Normals[corner] = receiverInterpolatePrecise(triangle.Normals, vertex.bary)

			for axis := range 2 {
				var value float64
				for source := range 3 {
					value += float64(triangle.UV[source][axis]) * vertex.bary[source]
				}

				patch.UV[corner][axis] = float32(value)
			}
		}

		patches = append(patches, patch)
	}

	return patches, nil
}

func receiverInterpolatePrecise(vertices [3]Vec3, bary [3]float64) Vec3 {
	var result Vec3

	for axis := range 3 {
		var value float64
		for vertex := range 3 {
			value += float64(vertices[vertex][axis]) * bary[vertex]
		}

		result[axis] = float32(value)
	}

	return result
}

// receiverProjectedBaryArea is only used for caller-provided barycentric pieces.
// Prepared footprints use original clipped UV areas through receiverClippedPatches.
//
//nolint:mnd // A two-dimensional determinant is twice triangle area.
func receiverProjectedBaryArea(a, b, c Vec3, first, second int) float64 {
	return math.Abs((float64(b[first])-float64(a[first]))*(float64(c[second])-float64(a[second]))-
		(float64(b[second])-float64(a[second]))*(float64(c[first])-float64(a[first]))) * .5
}
