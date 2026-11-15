//nolint:varnamelen // Coplanar surface tests use conventional geometric bounds notation.
package lightmapper

import "fmt"

// Emitting surfaces need a unique continuation-hit owner. Co-located delta
// sources remain additive; positive-area coincident emitting geometry is
// rejected instead of losing one source's emission in MIS tie resolution.
//
//nolint:gosec // Admitted emitter arrays contain at most2^20 records, fitting the traversal index.
func (p *PreparedScene) validateSurfaceOwnership() error {
	for index, emitter := range p.source.Emitters {
		if !finiteEmitter(emitter.Kind) {
			continue
		}

		lo, hi, _ := p.emitterBounds(emitter)
		if err := p.visitEmitterBounds(lo, hi, func(other uint32) error {
			if other <= uint32(index) || !finiteEmitter(p.source.Emitters[other].Kind) {
				return nil
			}

			if overlappingSurfaces(p.emitterSurface(emitter), p.emitterSurface(p.source.Emitters[other]), p.source.MetresPerUnit) {
				return fmt.Errorf(
					"emitting surfaces %d and %d share positive-area support; give each surface a unique emission owner: %w",
					emitter.ID,
					p.source.Emitters[other].ID,
					ErrUnsupported,
				)
			}

			return nil
		}); err != nil {
			return err
		}

		if emitter.Kind == Sphere {
			continue
		}

		if err := p.visitGeometryBounds(lo, hi, func(triangle uint32) error {
			if emitter.Kind == EmissiveTriangle && triangle == emitter.Triangle {
				return nil
			}

			if overlappingSurfaces(p.emitterSurface(emitter), triangleSurface(p.source.Triangles[triangle]), p.source.MetresPerUnit) {
				return fmt.Errorf(
					"emitter %d shares positive-area support with triangle %d; use that triangle's emission or separate the analytic source: %w",
					emitter.ID,
					p.source.Triangles[triangle].ID,
					ErrUnsupported,
				)
			}

			return nil
		}); err != nil {
			return err
		}
	}

	return nil
}

func finiteEmitter(kind EmitterKind) bool {
	return kind == Rectangle || kind == Disk || kind == Sphere || kind == EmissiveTriangle
}
func boundsOverlap(a, b, c, d Vec3) bool {
	for axis := range 3 {
		if b[axis] < c[axis] || d[axis] < a[axis] {
			return false
		}
	}

	return true
}

func (p *PreparedScene) visitEmitterBounds(lo, hi Vec3, visit func(uint32) error) error {
	if len(p.lightNodes) == 0 {
		return nil
	}

	var stack [64]uint32

	size := 1
	for size > 0 {
		size--

		node := p.lightNodes[stack[size]]
		if !boundsOverlap(lo, hi, node.Min, node.Max) {
			continue
		}

		if node.Count == 0 {
			stack[size] = node.Left
			stack[size+1] = node.Right
			size += 2

			continue
		}

		if err := visit(p.lightOrder[node.First]); err != nil {
			return err
		}
	}

	return nil
}
func (p *PreparedScene) visitGeometryBounds(lo, hi Vec3, visit func(uint32) error) error {
	if len(p.nodes) == 0 {
		return nil
	}

	var stack [64]uint32

	size := 1
	for size > 0 {
		size--

		node := p.nodes[stack[size]]
		if !boundsOverlap(lo, hi, node.Min, node.Max) {
			continue
		}

		if node.Count == 0 {
			stack[size] = node.Left
			stack[size+1] = node.Right
			size += 2

			continue
		}

		for _, triangle := range p.order[node.First : node.First+node.Count] {
			if err := visit(triangle); err != nil {
				return err
			}
		}
	}

	return nil
}

type overlapSurface struct {
	center, normal Vec3
	polygon        []Vec3
	radius         float32
	sphere         bool
}

func triangleSurface(triangle Triangle) overlapSurface {
	return overlapSurface{
		center:  triangle.Positions[0],
		normal:  Normalize(Cross(Sub(triangle.Positions[1], triangle.Positions[0]), Sub(triangle.Positions[2], triangle.Positions[0]))),
		polygon: triangle.Positions[:],
	}
}
func (p *PreparedScene) emitterSurface(emitter Emitter) overlapSurface {
	shape := overlapSurface{
		center: emitter.Position,
		normal: Normalize(emitter.Direction),
		radius: emitter.Radius,
		sphere: emitter.Kind == Sphere,
	}
	if emitter.Kind == EmissiveTriangle {
		return triangleSurface(p.source.Triangles[emitter.Triangle])
	}

	if emitter.Kind == Rectangle {
		shape.radius = 0
		t := Scale(emitter.Tangent, emitter.Size[0]*.5)
		b := Scale(Cross(emitter.Direction, emitter.Tangent), emitter.Size[1]*.5)
		shape.polygon = []Vec3{
			Sub(Sub(shape.center, t), b),
			Sub(Add(shape.center, t), b),
			Add(Add(shape.center, t), b),
			Add(Sub(shape.center, t), b),
		}
	}

	return shape
}
