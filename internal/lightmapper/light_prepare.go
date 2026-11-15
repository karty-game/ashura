package lightmapper

import "math"

func validateExplicitEmitter(emitter Emitter, duplicate bool) error {
	if duplicate ||
		!positiveVec(emitter.Intensity) ||
		!validVec(emitter.Position) ||
		!validVec(emitter.Direction) ||
		!validVec(emitter.Tangent) ||
		!finite(emitter.Radius) ||
		!finite(emitter.Range) ||
		!finite(emitter.InnerCos) ||
		!finite(emitter.OuterCos) ||
		emitter.Range < 0 ||
		emitter.Radius < 0 ||
		emitter.Model > LegacyRangeV1 {
		return ErrInvalid
	}

	return validateEmitterShapeFields(emitter)
}

func normalizeEmitterDirection(emitter *Emitter) error {
	if emitter.Kind == Spot || emitter.Kind == Rectangle || emitter.Kind == Disk || emitter.Kind == Directional || emitter.Kind == Sun {
		if math.Abs(float64(Length(emitter.Direction)-1)) > float64(canonicalUnitTolerance) {
			emitter.Direction = Normalize(emitter.Direction)
		}

		if emitter.Direction == (Vec3{}) {
			return ErrInvalid
		}
	}

	return nil
}

func validateEmitterExtent(emitter Emitter) error {
	if emitter.Kind == Spot && (emitter.OuterCos < -1 || emitter.InnerCos > 1 || emitter.InnerCos < emitter.OuterCos) {
		return ErrInvalid
	}

	if emitter.Kind == Rectangle && (!finite(emitter.Size[0]) || !finite(emitter.Size[1]) || emitter.Size[0] <= 0 || emitter.Size[1] <= 0) {
		return ErrInvalid
	}

	if (emitter.Kind == Disk || emitter.Kind == Sphere || emitter.Kind == Sun) && emitter.Radius <= 0 {
		return ErrInvalid
	}

	if emitter.Kind == Sun && emitter.Radius >= lightPi/2 {
		return ErrInvalid
	}

	if emitter.Kind == Sun && (sunCap(emitter.Radius) < math.Float32frombits(0x00800000) || !finite(1/(2*lightPi*sunCap(emitter.Radius)))) {
		return ErrBounds
	}

	return nil
}

func normalizeEmitterTangent(emitter *Emitter) {
	if emitter.Kind != Rectangle && emitter.Kind != Disk {
		return
	}

	if math.Abs(float64(Dot(emitter.Tangent, emitter.Direction))) > float64(canonicalUnitTolerance)*float64(Length(emitter.Tangent)) {
		emitter.Tangent = Sub(emitter.Tangent, Scale(emitter.Direction, Dot(emitter.Tangent, emitter.Direction)))
	}

	if math.Abs(float64(Length(emitter.Tangent)-1)) > float64(canonicalUnitTolerance) {
		emitter.Tangent = Normalize(emitter.Tangent)
	}

	if emitter.Tangent == (Vec3{}) {
		emitter.Tangent, _ = tangentFrame(emitter.Direction)
	}
}

//nolint:gosec // Scene admission caps source and triangle indices below the signed emitter-index representation.
func (prepared *PreparedScene) addMaterialEmitters(ids map[uint64]bool) {
	source := &prepared.source
	for index, triangle := range source.Triangles {
		material := source.Materials[triangle.Material]
		if maxComponent(material.Emission) > 0 && prepared.triangleEmitters[index] < 0 {
			identity := triangle.ID ^ implicitEmitterNamespace
			for ids[identity] {
				identity++
			}

			ids[identity] = true
			prepared.triangleEmitters[index] = int32(len(source.Emitters))
			source.Emitters = append(
				source.Emitters,
				Emitter{
					ID:        identity,
					Kind:      EmissiveTriangle,
					Intensity: material.Emission,
					Triangle:  uint32(index),
					TwoSided:  material.DoubleSided,
				},
			)
		}
	}
}

//nolint:gosec // Explicit and implicit source arrays remain below 2^21 entries before the final admission cap.
func (prepared *PreparedScene) addEnvironmentEmitter(ids map[uint64]bool) {
	source := &prepared.source
	if prepared.environmentEmitter < 0 &&
		(source.Environment.Texture > 0 || maxComponent(source.Environment.Sky) > 0 || maxComponent(source.Environment.Ground) > 0) {
		prepared.environmentEmitter = int32(len(source.Emitters))

		identity := uint64(math.MaxUint64)
		for ids[identity] {
			identity--
		}

		source.Emitters = append(source.Emitters, Emitter{ID: identity, Kind: EnvironmentSource, Intensity: Vec3{1, 1, 1}})
	}
}
