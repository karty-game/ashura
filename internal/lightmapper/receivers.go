package lightmapper

import (
	"errors"
	"fmt"
	"math"
)

// FootprintPiece is a clipped UV1 polygon with barycentric vertices. Area is in
// UV1 texel units, never world area; Triangle indexes the prepared scene.
type FootprintPiece struct {
	Triangle    uint32
	Barycentric []Vec3
	Area        float32
}

// Receiver retains a luxel's real covered support. Guide fields are not a
// replacement for sampling Pieces. Front and back records have independent IDs.
type Receiver struct {
	ID                                         uint64
	Pixel, Chart                               uint32
	Side                                       int32
	Pieces                                     []FootprintPiece
	Patches                                    []ReceiverPatch // Optional world-space support, independent of visibility triangulation.
	Coverage                                   float32
	Position, GeometricNormal, Normal, Tangent Vec3
	Handedness                                 float32
}

// SurfaceSample is one uniform covered-UV-area sample, with its exact frame.
// PDF is measured per covered texel area. The normalized box-filter estimator
// cancels this density; it is retained for alternate proposal implementations.
type SurfaceSample struct {
	Position, GeometricNormal, Normal, Tangent Vec3
	UV                                         Vec2
	Handedness                                 float32
	Triangle, Material                         uint32
	Coverage, PDF                              float32
}

// ReceiverPatch is a fan triangle used by GPU footprint samplers. Its Area is
// UV1 area even when its world-space area differs from another patch.
type ReceiverPatch struct {
	Positions, Normals       [3]Vec3
	UV                       [3]Vec2
	Area                     float32
	Triangle, Material       uint32
	GeometricNormal, Tangent Vec3
	RawTangent, RawBitangent Vec3
	Handedness               float32
}

var errReceiver = errors.New("lightmapper: invalid receiver footprint")

// ReceiverPatches triangulates exact clipped support for packed GPU records.
func ReceiverPatches(scene *PreparedScene, receiver Receiver) ([]ReceiverPatch, error) {
	if len(receiver.Patches) > 0 {
		return receiver.Patches, nil
	}

	if scene == nil {
		return nil, errReceiver
	}

	return receiverPatches(scene.Source().Triangles, receiver)
}

func receiverPatches(triangles []Triangle, receiver Receiver) ([]ReceiverPatch, error) {
	if len(receiver.Patches) > 0 {
		return receiver.Patches, nil
	}

	var (
		output []ReceiverPatch
		area   float32
	)

	for _, piece := range receiver.Pieces {
		if int(piece.Triangle) >= len(triangles) || len(piece.Barycentric) < 3 || piece.Area <= 0 {
			return nil, errReceiver
		}

		triangle := triangles[piece.Triangle]
		frame := receiverPatchSample(
			ReceiverPatch{Positions: triangle.Positions, Normals: triangle.Normals, UV: triangle.UV},
			Vec3{1.0 / 3, 1.0 / 3, 1.0 / 3},
			1,
		)
		rawTangent, rawBitangent := receiverRawFrame(triangle.Positions, triangle.UV)
		// An affine barycentric map preserves relative polygon fan areas.
		fans, fanTotal, err := receiverFans(piece, triangle.ID)
		if err != nil {
			return nil, err
		}

		for fanIndex, fanArea := range fans {
			if fanArea == 0 {
				continue
			}

			patch := ReceiverPatch{
				Area:            piece.Area * float32(fanArea/fanTotal),
				Triangle:        piece.Triangle,
				Material:        triangle.Material,
				GeometricNormal: frame.GeometricNormal,
				Tangent:         frame.Tangent,
				Handedness:      frame.Handedness,
				RawTangent:      rawTangent, RawBitangent: rawBitangent,
			}
			for corner, bary := range [3]Vec3{piece.Barycentric[0], piece.Barycentric[fanIndex+1], piece.Barycentric[fanIndex+2]} {
				if !receiverValidBary(bary) {
					return nil, errReceiver
				}

				patch.Positions[corner] = receiverInterpolate(triangle.Positions, bary)
				patch.Normals[corner] = receiverInterpolate(triangle.Normals, bary)

				for axis := range 2 {
					for vertex := range 3 {
						patch.UV[corner][axis] += triangle.UV[vertex][axis] * bary[vertex]
					}
				}
			}

			output = append(output, patch)
			area += patch.Area
		}
	}

	if !receiverFinite(area) || area <= 0 || receiver.Coverage <= 0 || math.Abs(float64(area-receiver.Coverage)) > 1e-4*float64(area) {
		return nil, errReceiver
	}

	return output, nil
}

// SampleReceiver samples one fan according to UV area, then samples uniformly
// inside it. All three uniforms belong to [0,1); scheduling cannot change them.
//
//nolint:varnamelen // u/v/w denote three independent uniform variates for area and triangle sampling.
func SampleReceiver(scene *PreparedScene, receiver Receiver, u, v, w float32) (SurfaceSample, error) {
	if scene == nil || !receiverUniform(u) || !receiverUniform(v) || !receiverUniform(w) {
		return SurfaceSample{}, errReceiver
	}

	patches, err := ReceiverPatches(scene, receiver)
	if err != nil {
		return SurfaceSample{}, err
	}

	target := u * receiver.Coverage

	patch := patches[len(patches)-1]
	for _, candidate := range patches {
		target -= candidate.Area
		if target < 0 {
			patch = candidate

			break
		}
	}

	root := float32(math.Sqrt(float64(v)))
	bary := Vec3{1 - root, root * (1 - w), root * w}
	sample := receiverPatchSample(patch, bary, receiver.Side)
	sample.Coverage = receiver.Coverage
	sample.PDF = 1 / receiver.Coverage

	return sample, nil
}

func receiverGuide(triangles []Triangle, receiver *Receiver) error {
	patches, err := receiverPatches(triangles, *receiver)
	if err != nil {
		return err
	}

	var (
		normal, tangent, geometric Vec3
		handed                     float32
	)

	for _, patch := range patches {
		sample := receiverPatchSample(patch, Vec3{1.0 / 3, 1.0 / 3, 1.0 / 3}, receiver.Side)
		scale := patch.Area / receiver.Coverage
		receiver.Position = Add(receiver.Position, Scale(sample.Position, scale))
		normal = Add(normal, Scale(sample.Normal, scale))
		tangent = Add(tangent, Scale(sample.Tangent, scale))
		geometric = Add(geometric, Scale(sample.GeometricNormal, scale))
		handed += sample.Handedness * scale
	}

	receiver.Normal = Normalize(normal)
	receiver.GeometricNormal = Normalize(geometric)
	receiver.Tangent = Normalize(Sub(tangent, Scale(receiver.Normal, Dot(receiver.Normal, tangent))))

	receiver.Handedness = 1
	if handed < 0 {
		receiver.Handedness = -1
	}

	if Dot(receiver.Normal, receiver.Normal) == 0 || Dot(receiver.GeometricNormal, receiver.GeometricNormal) == 0 {
		return fmt.Errorf("zero guide normal: normal %v, geometric %v, pieces %v: %w", normal, geometric, receiver.Pieces, errReceiver)
	}

	return nil
}

//nolint:mnd // Near-singular UV determinants and near-parallel fallback axes use explicit geometric tolerances.
func receiverPatchSample(patch ReceiverPatch, bary Vec3, side int32) SurfaceSample {
	sample := SurfaceSample{Position: receiverInterpolate(patch.Positions, bary), Triangle: patch.Triangle, Material: patch.Material}

	if Dot(patch.GeometricNormal, patch.GeometricNormal) > 0 {
		sample.GeometricNormal = patch.GeometricNormal
	} else {
		sample.GeometricNormal = Normalize(Cross(Sub(patch.Positions[1], patch.Positions[0]), Sub(patch.Positions[2], patch.Positions[0])))
	}

	sample.Normal = Normalize(receiverInterpolate(patch.Normals, bary))
	if Dot(sample.Normal, sample.Normal) == 0 {
		sample.Normal = sample.GeometricNormal
	}

	for axis := range 2 {
		for vertex := range 3 {
			sample.UV[axis] += patch.UV[vertex][axis] * bary[vertex]
		}
	}

	var rawTangent, rawBitangent Vec3
	if Dot(patch.RawTangent, patch.RawTangent) > 0 {
		rawTangent, rawBitangent = patch.RawTangent, patch.RawBitangent
	} else {
		deltaA, deltaB := Sub(patch.Positions[1], patch.Positions[0]), Sub(patch.Positions[2], patch.Positions[0])
		uvA, uvB := Vec2{
			patch.UV[1][0] - patch.UV[0][0],
			patch.UV[1][1] - patch.UV[0][1],
		}, Vec2{
			patch.UV[2][0] - patch.UV[0][0],
			patch.UV[2][1] - patch.UV[0][1],
		}

		determinant := uvA[0]*uvB[1] - uvA[1]*uvB[0]
		if math.Abs(float64(determinant)) > 1e-20 {
			rawTangent = Scale(Sub(Scale(deltaA, uvB[1]), Scale(deltaB, uvA[1])), 1/determinant)
			rawBitangent = Scale(Sub(Scale(deltaB, uvA[0]), Scale(deltaA, uvB[0])), 1/determinant)
		}
	}

	sample.Tangent = Normalize(Sub(rawTangent, Scale(sample.Normal, Dot(sample.Normal, rawTangent))))
	if Dot(sample.Tangent, sample.Tangent) == 0 {
		axis := Vec3{0, 1, 0}
		if math.Abs(float64(sample.Normal[1])) > .9 {
			axis = Vec3{1, 0, 0}
		}

		sample.Tangent = Normalize(Cross(axis, sample.Normal))
	}

	sample.Handedness = 1
	if Dot(Cross(sample.Normal, sample.Tangent), rawBitangent) < 0 {
		sample.Handedness = -1
	}

	if side < 0 {
		sample.Normal = Scale(sample.Normal, -1)
		sample.GeometricNormal = Scale(sample.GeometricNormal, -1)
		sample.Tangent = Scale(sample.Tangent, -1)
		sample.Handedness = -sample.Handedness
	}

	return sample
}

func receiverInterpolate(vertices [3]Vec3, bary Vec3) Vec3 {
	var result Vec3
	for i := range 3 {
		result = Add(result, Scale(vertices[i], bary[i]))
	}

	return result
}

//nolint:mnd // The two-dimensional determinant is twice triangle area.
func receiverBaryArea(a, b, c Vec3) float64 {
	return math.Abs(float64(b[1]-a[1])*float64(c[2]-a[2])-float64(b[2]-a[2])*float64(c[1]-a[1])) * .5
}
func receiverFinite(value float32) bool {
	return !math.IsNaN(float64(value)) && !math.IsInf(float64(value), 0)
}
func receiverUniform(value float32) bool { return receiverFinite(value) && value >= 0 && value < 1 }

//nolint:mnd // The tolerance admits float32 barycentric interpolation roundoff.
func receiverValidBary(bary Vec3) bool {
	var sum float32

	for _, v := range bary {
		if !receiverFinite(v) || v < -.00001 || v > 1.00001 {
			return false
		}

		sum += v
	}

	return math.Abs(float64(sum-1)) < .0001
}

//nolint:mnd,varnamelen // a/b and u/v are edge and UV differentials; the determinant tolerance detects singular texture coordinates.
func receiverRawFrame(positions [3]Vec3, uv [3]Vec2) (Vec3, Vec3) {
	a, b := Sub(positions[1], positions[0]), Sub(positions[2], positions[0])
	u, v := Vec2{uv[1][0] - uv[0][0], uv[1][1] - uv[0][1]}, Vec2{uv[2][0] - uv[0][0], uv[2][1] - uv[0][1]}

	determinant := u[0]*v[1] - u[1]*v[0]
	if math.Abs(float64(determinant)) <= 1e-20 {
		return Vec3{}, Vec3{}
	}

	return Scale(Sub(Scale(a, v[1]), Scale(b, u[1])), 1/determinant), Scale(Sub(Scale(b, u[0]), Scale(a, v[0])), 1/determinant)
}

//nolint:mnd // Choose a common two-coordinate projection from the three barycentric axes.
func receiverFans(piece FootprintPiece, triangleID uint64) ([]float64, float64, error) {
	var fanTotal float64

	fans := make([]float64, len(piece.Barycentric)-2)

	for first := range 3 {
		second := (first + 1) % 3
		candidate := make([]float64, len(fans))

		var total float64

		for index := range candidate {
			candidate[index] = receiverProjectedBaryArea(
				piece.Barycentric[0],
				piece.Barycentric[index+1],
				piece.Barycentric[index+2],
				first,
				second,
			)
			total += candidate[index]
		}

		if total > fanTotal {
			fans, fanTotal = candidate, total
		}
	}

	if fanTotal <= 0 {
		return nil, 0, fmt.Errorf(
			"triangle %d (id %d): barycentric polygon %v has no representable fan area (coverage %g): %w",
			piece.Triangle,
			triangleID,
			piece.Barycentric,
			piece.Area,
			errReceiver,
		)
	}

	return fans, fanTotal, nil
}
