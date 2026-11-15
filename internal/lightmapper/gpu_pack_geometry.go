//go:build !js

package lightmapper

// Stable primitive priority reproduces the original right-first winner when
// equal-distance triangles are encountered in a different octant traversal.
func gpuGeometryPriorities(scene *PreparedScene) []uint32 {
	priorities := make([]uint32, len(scene.order))

	var rank uint32

	var walk func(uint32)

	walk = func(index uint32) {
		node := scene.nodes[index]
		if node.Count == 0 {
			walk(node.Right)
			walk(node.Left)

			return
		}

		for slot := node.First; slot < node.First+node.Count; slot++ {
			priorities[slot] = rank
			rank++
		}
	}

	if len(scene.nodes) > 0 {
		walk(0)
	}

	return priorities
}
