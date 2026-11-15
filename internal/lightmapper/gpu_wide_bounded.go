//go:build !js

package lightmapper

import (
	"errors"
	"fmt"
)

// balancedFrontier expands the largest remaining binary subtree first. micro
// halves every range, so a full frontier of eight contains at most ceil(n/8)
// primitives per child. Leaves need no expansion. This avoids imposing a new
// scene-size or hierarchy-depth restriction on an otherwise admitted scene.
func (b *gpuWideBuilder) balancedFrontier(id int) []int {
	children := []int{id}
	for len(children) < 8 {
		largest := -1
		for i, child := range children {
			if len(b.nodes[child].tris) == 0 && (largest < 0 || b.nodes[child].count > b.nodes[children[largest]].count) {
				largest = i
			}
		}

		if largest < 0 {
			break
		}

		n := b.nodes[children[largest]]
		children[largest] = n.left
		children = append(children, n.right)
	}

	return children
}

func (b *gpuWideBuilder) buildBounded(root, capacity, wordBudget int) error {
	b.dp(root)
	b.emit(root, 0, 1)

	if b.err != nil && !errors.Is(b.err, ErrBounds) {
		return b.err
	}

	if b.err == nil && b.depth <= capacity && len(b.packed) <= wordBudget {
		return nil
	}
	// The SAH topology is only a scheduling choice. Rebuild exceptional deep
	// trees from the same source identities, retaining all geometry and the
	// original tie ranks. Ordinary scenes retain their original packed bytes.
	b.nodes = nil
	b.packed = make([]uint32, 20)
	b.order = nil
	b.depth = 0
	b.balanced = true
	b.err = nil
	root = b.micro(b.oldOrder)
	b.emit(root, 0, 1)

	if b.err != nil {
		return b.err
	}

	if len(b.packed) > wordBudget {
		return ErrBounds
	}

	if b.depth > capacity {
		return fmt.Errorf("balanced hierarchy depth %d exceeds proved capacity %d: %w", b.depth, capacity, ErrInvalid)
	}

	return nil
}
