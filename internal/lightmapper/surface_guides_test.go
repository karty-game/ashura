package lightmapper

import "testing"

func TestCompactSurfaceGuidesPreserveFramesAndVisibilityRays(t *testing.T) {
	t.Parallel()

	receivers := []Receiver{
		{ID: 1, Position: Vec3{1, 2, 3}, Normal: Vec3{0, 1, 0}, GeometricNormal: Vec3{0, 0, 1}, Tangent: Vec3{1, 0, 0}, Handedness: 1},
		{ID: 2, Position: Vec3{1.125, 2, 3}, Normal: Vec3{0, 1, 0}, GeometricNormal: Vec3{0, 0, 1}, Tangent: Vec3{1, 0, 0}, Handedness: 1},
		{
			ID:              3,
			Position:        Vec3{1.125, 2, 3},
			Normal:          Vec3{0, 1, 0},
			GeometricNormal: Vec3{0, 1, 0},
			Tangent:         Vec3{1, 0, 0},
			Handedness:      -1,
			Side:            1,
		},
		{ID: 4, Position: Vec3{100000, 2, 3}, Normal: Vec3{1, 0, 0}, GeometricNormal: Vec3{1, 0, 0}},
	}
	guides := surfaceSupportReceivers(receivers)
	options := ReconstructionOptions{Radius: .25, PlaneTolerance: .025, NormalCos: .95, TangentCos: .95, NoiseScale: 128, Strength: 1}

	for index, receiver := range receivers {
		guide := guides[index]
		if guide.id != receiver.ID || guide.guide != reconstructionReceiverGuide(receiver) ||
			guide.geometricNormal != receiver.GeometricNormal {
			t.Fatal("compact guide lost a physical frame", index)
		}

		for next, neighbor := range receivers {
			other := guides[next]
			if reconstructionGuidesEligible(guide.guide, other.guide, options) != surfaceSupportEligible(receiver, neighbor, &options) {
				t.Fatal("changed physical support eligibility", index, next)
			}

			for _, explicit := range []float32{0, .001} {
				actual := surfaceGuideVisibilityRay(
					guide.guide.Position,
					other.guide.Position,
					guide.geometricNormal,
					other.geometricNormal,
					explicit,
				)
				expected := scalarSurfaceRay(receiver, neighbor, explicit)

				if actual != expected {
					t.Fatal("changed visibility ray bits", index, next, actual, expected)
				}
			}
		}
	}
}

func scalarSurfaceRay(first, second Receiver, explicit float32) Ray {
	tolerance := surfaceTolerance(first.Position, second.Position, explicit)
	origin := Add(first.Position, Scale(first.GeometricNormal, tolerance))
	destination := Add(second.Position, Scale(second.GeometricNormal, tolerance))
	delta := Sub(destination, origin)
	distance := Length(delta)

	if distance <= 2*tolerance {
		return Ray{}
	}

	return Ray{Origin: origin, Direction: Scale(delta, 1/distance), Min: tolerance, Max: distance - tolerance}
}
