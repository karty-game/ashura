package lightmapper

import "fmt"

// validateEmitterShapeFields rejects active fields that the selected emission
// model cannot evaluate. Transform metadata may remain on symmetric/global
// sources; finite radius and attenuation must never silently become delta light.
//
//nolint:cyclop // Each emitter kind rejects unsupported active fields before constructing its sampling representation.
func validateEmitterShapeFields(emitter Emitter) error {
	if !finite(emitter.Size[0]) || !finite(emitter.Size[1]) || !finite(emitter.Radius) || !finite(emitter.Range) {
		return ErrInvalid
	}

	hasSize := emitter.Size != (Vec2{})
	hasRadius := emitter.Radius != 0

	hasRange := emitter.Range != 0
	if emitter.Kind != Spot && (emitter.InnerCos != 0 || emitter.OuterCos != 0) {
		return fmt.Errorf("angular falloff is supported only by spot sources: %w", ErrUnsupported)
	}

	switch emitter.Kind {
	case Point, Spot:
		if hasSize || hasRadius {
			return fmt.Errorf(
				"point and spot sources are delta emitters; choose a finite disk or sphere source for radius: %w",
				ErrUnsupported,
			)
		}
	case Rectangle:
		if hasRadius {
			return fmt.Errorf("rectangle sources use Size instead of Radius: %w", ErrUnsupported)
		}
	case Disk, Sphere:
		if hasSize {
			return fmt.Errorf("disk and sphere sources use Radius instead of Size: %w", ErrUnsupported)
		}
	case EmissiveTriangle:
		if hasSize || hasRadius {
			return fmt.Errorf("triangle emission uses its geometry rather than Size or Radius: %w", ErrUnsupported)
		}
	case Directional:
		if hasSize || hasRadius || hasRange {
			return fmt.Errorf("delta directional sources have no finite extent or range; choose Sun for angular radius: %w", ErrUnsupported)
		}
	case Sun:
		if hasSize || hasRange {
			return fmt.Errorf("sun sources use angular Radius and have no finite Size or Range: %w", ErrUnsupported)
		}
	case EnvironmentSource:
		if hasSize || hasRadius || hasRange {
			return fmt.Errorf("environment sources have no finite Size, Radius or Range: %w", ErrUnsupported)
		}
	default:
		return ErrUnsupported
	}

	return nil
}
