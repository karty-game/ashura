//nolint:varnamelen // Hierarchy construction and traversal retain conventional node and bounds notation.
package lightmapper

import (
	"math"
	"sort"
)

const canonicalUnitTolerance float32 = 1e-5
const implicitEmitterNamespace uint64 = 0xf000000000000000
const lightDistanceRegularizer float32 = 1e-4

// prepareEmitters admits explicit sources before appending implicit material and
// environment sources. Index assignments retain that source order.
func (p *PreparedScene) prepareEmitters() error {
	ids := make(map[uint64]bool, len(p.source.Emitters))

	source := &p.source
	for index := range source.Emitters {
		emitter := &source.Emitters[index]
		if err := validateExplicitEmitter(*emitter, ids[emitter.ID]); err != nil {
			return err
		}

		ids[emitter.ID] = true
		if emitter.Kind < Point || emitter.Kind > EnvironmentSource {
			return ErrUnsupported
		}

		if emitter.Kind == EnvironmentSource {
			if p.environmentEmitter >= 0 {
				return ErrInvalid
			}

			p.environmentEmitter = int32(index)
		}

		if emitter.Model == LegacyRangeV1 && (emitter.Kind != Point && emitter.Kind != Spot || emitter.Range <= 0) {
			return ErrUnsupported
		}

		if err := normalizeEmitterDirection(emitter); err != nil {
			return err
		}

		if err := validateEmitterExtent(*emitter); err != nil {
			return err
		}

		normalizeEmitterTangent(emitter)

		if emitter.Kind == EmissiveTriangle {
			if uint64(emitter.Triangle) >= uint64(len(source.Triangles)) || p.triangleEmitters[emitter.Triangle] >= 0 {
				return ErrInvalid
			}

			p.triangleEmitters[emitter.Triangle] = int32(index)
		}
	}

	p.addMaterialEmitters(ids)
	p.addEnvironmentEmitter(ids)

	if len(source.Emitters) > 1<<20 {
		return ErrBounds
	}

	return nil
}

//nolint:exhaustive,mnd //  Point bounds are initialized at their position; omitted power cases retain their declared intensity. Point/environment flux integrates over the full 4pi solid angle.
func (p *PreparedScene) emitterBounds(e Emitter) (Vec3, Vec3, float32) {
	area := float32(1)

	lo, hi := e.Position, e.Position
	switch e.Kind {
	case Rectangle:
		r := Scale(e.Tangent, e.Size[0]/2)
		s := Scale(Cross(e.Direction, e.Tangent), e.Size[1]/2)
		extent := Vec3{
			float32(math.Abs(float64(r[0]))) + float32(math.Abs(float64(s[0]))),
			float32(math.Abs(float64(r[1]))) + float32(math.Abs(float64(s[1]))),
			float32(math.Abs(float64(r[2]))) + float32(math.Abs(float64(s[2]))),
		}
		lo, hi = Sub(e.Position, extent), Add(e.Position, extent)
		area = e.Size[0] * e.Size[1] * p.source.MetresPerUnit * p.source.MetresPerUnit
	case Disk, Sphere:
		extent := Vec3{e.Radius, e.Radius, e.Radius}
		lo, hi = Sub(e.Position, extent), Add(e.Position, extent)

		area = lightPi * e.Radius * e.Radius * p.source.MetresPerUnit * p.source.MetresPerUnit
		if e.Kind == Sphere {
			area *= 4
		}
	case EmissiveTriangle:
		t := p.source.Triangles[e.Triangle]
		lo = minVec(t.Positions[0], minVec(t.Positions[1], t.Positions[2]))
		hi = maxVec(t.Positions[0], maxVec(t.Positions[1], t.Positions[2]))
		area = Length(
			Cross(Sub(t.Positions[1], t.Positions[0]), Sub(t.Positions[2], t.Positions[0])),
		) * .5 * p.source.MetresPerUnit * p.source.MetresPerUnit
	case Directional, Sun, EnvironmentSource:
		large := float32(1e6) / p.source.MetresPerUnit
		lo, hi = Vec3{-large, -large, -large}, Vec3{large, large, large}
	}

	power := luminance(e.Intensity)
	switch e.Kind {
	case Rectangle, Disk, Sphere, EmissiveTriangle:
		power *= area * lightPi
		if e.TwoSided {
			power *= 2
		}
	case Point, Spot:
		power *= 4 * lightPi

		if e.Model == LegacyRangeV1 {
			r := e.Range * p.source.MetresPerUnit
			power *= max(1, r*r)
		}
	case EnvironmentSource:
		power = max(luminance(p.source.Environment.Sky), luminance(p.source.Environment.Ground))
		if p.source.Environment.Texture > 0 {
			power = 0
			for _, v := range p.source.Textures[p.source.Environment.Texture-1].Pixels {
				power += luminance(v)
			}

			power /= float32(len(p.source.Textures[p.source.Environment.Texture-1].Pixels))
		}

		power *= 4 * lightPi
	}

	return lo, hi, max(power, 1e-12)
}

//nolint:funlen,gosec,mnd // One deterministic bounded hierarchy build keeps partition, parent and leaf updates together; binary splits use fixed vector arities.
func (p *PreparedScene) buildLights() {
	n := len(p.source.Emitters)
	p.lightOrder = make([]uint32, n)

	p.lightLeaves = make([]uint32, n)
	for i := range n {
		p.lightOrder[i] = uint32(i)
	}

	lows, highs, powers := make([]Vec3, n), make([]Vec3, n), make([]float32, n)

	globals := make([]float32, n)
	for i, e := range p.source.Emitters {
		lows[i], highs[i], powers[i], globals[i] = p.lightNodeSupport(e)
	}

	if n == 0 {
		return
	}

	var build func(int, int, uint32) uint32

	build = func(first, end int, parent uint32) uint32 {
		index := uint32(len(p.lightNodes))
		p.lightNodes = append(p.lightNodes, LightNode{})
		p.lightParents = append(p.lightParents, parent)
		lo, hi := Vec3{math.MaxFloat32, math.MaxFloat32, math.MaxFloat32}, Vec3{-math.MaxFloat32, -math.MaxFloat32, -math.MaxFloat32}
		power := float32(0)
		globalPower := float32(0)
		finiteCount := 0

		for _, i := range p.lightOrder[first:end] {
			if !globalEmitter(p.source.Emitters[i].Kind) {
				lo = minVec(lo, lows[i])
				hi = maxVec(hi, highs[i])
				finiteCount++
			}

			power += powers[i]
			globalPower += globals[i]
		}

		lo, hi = maxFiniteBounds(lo, hi, finiteCount)

		node := LightNode{Min: lo, Max: hi, Power: power, GlobalPower: globalPower, ConeCos: -1}
		if end-first == 1 {
			node.First, node.Count = uint32(first), 1
			p.lightLeaves[p.lightOrder[first]] = index
			node.Direction, node.ConeCos = p.emitterCone(p.source.Emitters[p.lightOrder[first]])
		} else {
			d := Sub(hi, lo)

			axis := 0
			if d[1] > d[axis] {
				axis = 1
			}

			if d[2] > d[axis] {
				axis = 2
			}

			sort.SliceStable(p.lightOrder[first:end], func(a, b int) bool {
				ia, ib := p.lightOrder[first+a], p.lightOrder[first+b]

				ca, cb := lows[ia][axis]*.5+highs[ia][axis]*.5, lows[ib][axis]*.5+highs[ib][axis]*.5
				if ca == cb {
					return p.source.Emitters[ia].ID < p.source.Emitters[ib].ID
				}

				return ca < cb
			})
			middle := (first + end) / 2
			node.Left = build(first, middle, index)
			node.Right = build(middle, end, index)
			node.Direction, node.ConeCos = mergeLightCones(p.lightNodes[node.Left], p.lightNodes[node.Right])
		}

		p.lightNodes[index] = node

		return index
	}
	build(0, n, math.MaxUint32)
}

func lightWeight(position Vec3, node LightNode, meters, powerScale float32) float32 {
	global := node.GlobalPower / powerScale
	if node.Power == 0 {
		return global
	}

	delta := Vec3{
		max(node.Min[0]-position[0], position[0]-node.Max[0], 0),
		max(node.Min[1]-position[1], position[1]-node.Max[1], 0),
		max(node.Min[2]-position[2], position[2]-node.Max[2], 0),
	}
	extent := Sub(node.Max, node.Min)
	physicalDelta, physicalExtent := Scale(delta, meters), Scale(extent, meters)
	spatial := (node.Power / powerScale) / (Dot(physicalDelta, physicalDelta) + .25*Dot(physicalExtent, physicalExtent) + lightDistanceRegularizer)

	return global + spatial*lightConeImportance(position, node)
}

//nolint:mnd // An exactly balanced binary proposal uses probability one half.
func (p *PreparedScene) LightBranchProbability(position Vec3, index uint32) float32 {
	n := p.lightNodes[index]

	l, r := lightWeight(
		position,
		p.lightNodes[n.Left],
		p.source.MetresPerUnit,
		p.lightPowerScale(),
	), lightWeight(
		position,
		p.lightNodes[n.Right],
		p.source.MetresPerUnit,
		p.lightPowerScale(),
	)
	if l+r == 0 {
		return .5
	}

	return max(.03125, min(.96875, l/(l+r)))
}

// SampleLight is logarithmic in the admitted emitter count and returns the
// exact discrete mass, including hierarchy branch floors. No light is dropped.
func (p *PreparedScene) SampleLight(position Vec3, u float32) (uint32, float32) {
	if len(p.lightNodes) == 0 {
		return 0, 0
	}

	index, mass := uint32(0), float32(1)
	for p.lightNodes[index].Count == 0 {
		node := p.lightNodes[index]

		left := p.LightBranchProbability(position, index)
		if u < left {
			u /= left
			mass *= left
			index = node.Left
		} else {
			u = (u - left) / (1 - left)
			mass *= 1 - left
			index = node.Right
		}
	}

	return p.lightOrder[p.lightNodes[index].First], mass
}

func (p *PreparedScene) LightPMF(position Vec3, emitter uint32) float32 {
	if uint64(emitter) >= uint64(len(p.lightLeaves)) {
		return 0
	}

	index, mass := p.lightLeaves[emitter], float32(1)
	for index != 0 {
		parent := p.lightParents[index]

		left := p.LightBranchProbability(position, parent)
		if p.lightNodes[parent].Left == index {
			mass *= left
		} else {
			mass *= 1 - left
		}

		index = parent
	}

	return mass
}
