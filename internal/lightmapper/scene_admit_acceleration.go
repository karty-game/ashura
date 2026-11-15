package lightmapper

import "math"

//nolint:varnamelen // lo and hi are the conventional lower and upper vector bounds.
func validBounds(lo, hi Vec3) bool {
	if !validVec(lo) || !validVec(hi) {
		return false
	}

	for a := range 3 {
		if lo[a] > hi[a] {
			return false
		}
	}

	return true
}
func boundsContain(lo, hi, a, b Vec3) bool {
	for nodeIndex := range 3 {
		if lo[nodeIndex] > a[nodeIndex] || hi[nodeIndex] < b[nodeIndex] {
			return false
		}
	}

	return true
}
func validPermutation(order []uint32, count int) bool {
	if count < 0 || len(order) != count {
		return false
	}

	seen := make([]bool, count)
	for _, nodeIndex := range order {
		if uint64(nodeIndex) >= uint64(count) || seen[nodeIndex] {
			return false
		}

		seen[nodeIndex] = true
	}

	return true
}

func (p *PreparedScene) validatePreparedAcceleration() error {
	if !validPermutation(p.order, len(p.source.Triangles)) || !validPermutation(p.lightOrder, len(p.source.Emitters)) {
		return ErrInvalid
	}

	if err := p.validateGeometryNodes(); err != nil {
		return err
	}

	if err := p.validateLightNodes(); err != nil {
		return err
	}

	return p.validateEnvironmentCDF()
}

//nolint:gocognit,cyclop,funlen //  Recursive admission keeps bounded node ownership, containment and coverage checks together. Recursive validation keeps node bounds, unique triangle slots and reachability in one complete-tree admission.
func (p *PreparedScene) validateGeometryNodes() error {
	if len(p.order) == 0 {
		if len(p.nodes) != 0 {
			return ErrInvalid
		}

		return nil
	}

	if len(p.nodes) == 0 || len(p.nodes) > len(p.order)*2 {
		return ErrBounds
	}

	seen, slots := make([]bool, len(p.nodes)), make([]bool, len(p.order))

	var walk func(uint32, int) error

	walk = func(nodeIndex uint32, depth int) error {
		if uint64(nodeIndex) >= uint64(len(p.nodes)) || seen[nodeIndex] || depth >= 64 {
			return ErrInvalid
		}

		seen[nodeIndex] = true

		node := p.nodes[nodeIndex]
		if !validBounds(node.Min, node.Max) {
			return ErrInvalid
		}

		if node.Count == 0 {
			if node.Left == node.Right || uint64(node.Left) >= uint64(len(p.nodes)) || uint64(node.Right) >= uint64(len(p.nodes)) {
				return ErrInvalid
			}

			for _, child := range []uint32{node.Left, node.Right} {
				c := p.nodes[child]
				if !boundsContain(node.Min, node.Max, c.Min, c.Max) {
					return ErrInvalid
				}

				if err := walk(child, depth+1); err != nil {
					return err
				}
			}

			return nil
		}

		if node.Count > 8 || uint64(node.First)+uint64(node.Count) > uint64(len(p.order)) {
			return ErrBounds
		}

		for slotIndex := node.First; slotIndex < node.First+node.Count; slotIndex++ {
			if slots[slotIndex] {
				return ErrInvalid
			}

			slots[slotIndex] = true

			texture := p.source.Triangles[p.order[slotIndex]]
			for _, v := range texture.Positions {
				if !boundsContain(node.Min, node.Max, v, v) {
					return ErrInvalid
				}
			}
		}

		return nil
	}
	if err := walk(0, 0); err != nil {
		return err
	}

	for _, v := range seen {
		if !v {
			return ErrInvalid
		}
	}

	for _, v := range slots {
		if !v {
			return ErrInvalid
		}
	}

	return nil
}

//nolint:funlen,gocognit,gocyclo,cyclop //  Recursive admission keeps probabilities, parent links, support and leaf ownership together. Admit the complete light hierarchy, exact source ownership and per-kind probability support atomically.
func (p *PreparedScene) validateLightNodes() error {
	if len(p.lightOrder) == 0 {
		if len(p.lightNodes) != 0 || len(p.lightParents) != 0 || len(p.lightLeaves) != 0 {
			return ErrInvalid
		}

		return nil
	}

	if len(p.lightNodes) == 0 || len(p.lightNodes) > len(p.lightOrder)*2 || len(p.lightParents) != len(p.lightNodes) ||
		len(p.lightLeaves) != len(p.lightOrder) ||
		p.lightParents[0] != math.MaxUint32 {
		return ErrInvalid
	}

	seen, slots := make([]bool, len(p.lightNodes)), make([]bool, len(p.lightOrder))

	var walk func(uint32, int) error

	walk = func(nodeIndex uint32, depth int) error {
		if uint64(nodeIndex) >= uint64(len(p.lightNodes)) || seen[nodeIndex] || depth > 24 {
			return ErrInvalid
		}

		seen[nodeIndex] = true

		node := p.lightNodes[nodeIndex]
		if !validBounds(node.Min, node.Max) || !finite(node.Power) || node.Power < 0 || !globalPowerFinite(node) || !finite(node.ConeCos) ||
			node.ConeCos < -1 ||
			node.ConeCos > 1 ||
			!validVec(node.Direction) {
			return ErrInvalid
		}

		if node.ConeCos > -1 && (Length(node.Direction) < .999 || Length(node.Direction) > 1.001) {
			return ErrInvalid
		}

		if node.Count == 0 {
			if node.Left == node.Right || uint64(node.Left) >= uint64(len(p.lightNodes)) ||
				uint64(node.Right) >= uint64(len(p.lightNodes)) {
				return ErrInvalid
			}

			for _, child := range []uint32{node.Left, node.Right} {
				c := p.lightNodes[child]
				if p.lightParents[child] != nodeIndex || (lightNodeHasFinite(c) && !boundsContain(node.Min, node.Max, c.Min, c.Max)) ||
					float64(c.Power) > float64(node.Power)*1.0001 || float64(c.GlobalPower) > float64(node.GlobalPower)*1.0001 {
					return ErrInvalid
				}

				if err := walk(child, depth+1); err != nil {
					return err
				}
			}

			return nil
		}

		if node.Count != 1 || uint64(node.First) >= uint64(len(p.lightOrder)) || slots[node.First] {
			return ErrInvalid
		}

		slots[node.First] = true

		emitter := p.lightOrder[node.First]
		if p.lightLeaves[emitter] != nodeIndex {
			return ErrInvalid
		}

		isGlobal := globalEmitter(p.source.Emitters[emitter].Kind)
		if (isGlobal && (node.Power != 0 || node.GlobalPower <= 0)) ||
			(!isGlobal && (node.Power <= 0 || node.GlobalPower != 0)) {
			return ErrInvalid
		}

		lo, hi, _ := p.emitterBounds(p.source.Emitters[emitter])
		if !globalEmitter(p.source.Emitters[emitter].Kind) && !boundsContain(node.Min, node.Max, lo, hi) {
			return ErrInvalid
		}

		return nil
	}
	if err := walk(0, 0); err != nil {
		return err
	}

	for _, v := range seen {
		if !v {
			return ErrInvalid
		}
	}

	for _, v := range slots {
		if !v {
			return ErrInvalid
		}
	}

	return nil
}

func validEnvironmentCDF(cdf []float32) bool {
	if len(cdf) < 2 || cdf[0] != 0 || cdf[len(cdf)-1] != 1 {
		return false
	}

	for nodeIndex := 1; nodeIndex < len(cdf); nodeIndex++ {
		if !finite(cdf[nodeIndex]) || cdf[nodeIndex] <= cdf[nodeIndex-1] {
			return false
		}
	}

	return true
}
func (p *PreparedScene) validateEnvironmentCDF() error {
	if p.source.Environment.Texture == 0 {
		if len(p.environmentRows) != 0 || len(p.environmentColumns) != 0 {
			return ErrInvalid
		}

		return nil
	}

	texture := p.source.Textures[p.source.Environment.Texture-1]
	if uint64(len(p.environmentRows)) != uint64(texture.Height)+1 ||
		uint64(len(p.environmentColumns)) != uint64(texture.Height)*(uint64(texture.Width)+1) ||
		!validEnvironmentCDF(p.environmentRows) {
		return ErrInvalid
	}

	for y := range texture.Height {
		if !validEnvironmentCDF(p.environmentColumns[int(y)*(int(texture.Width)+1) : int(y+1)*(int(texture.Width)+1)]) {
			return ErrInvalid
		}
	}

	return nil
}
