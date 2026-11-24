package ui

import "github.com/karty-game/ashura/internal/uipaint"

// Mixed image/text controls retain contiguous texture runs in painter order.
// A paint-only update replaces its own runs in place when their count is stable.
func (v *View) updateBatches(node *Node, fallback *uipaint.Texture) {
	previous := len(node.batches)
	node.batches = node.batches[:0]

	for index, record := range node.paint {
		texture := record.texture
		if texture == nil {
			texture = fallback
		}

		last := len(node.batches) - 1
		if last >= 0 && node.batches[last].Texture == texture {
			node.batches[last].Count++
		} else {
			node.batches = append(node.batches, uipaint.Batch{First: node.slot + index, Count: 1, Texture: texture})
		}
	}

	// Empty popup/tooltip batches retain their ordering even before first paint.
	if len(node.batches) == 0 {
		node.batches = append(node.batches, uipaint.Batch{First: node.slot, Texture: fallback})
	}

	if len(node.batches) < previous {
		clear(node.batches[len(node.batches):previous])
	}

	if len(node.batches) != previous {
		v.batchesDirty = true
	}

	if !v.batchesDirty {
		copy(v.frame.Batches[node.batch:], node.batches)
	}
}

func (v *View) rebuildBatches() {
	previous := len(v.frame.Batches)

	v.frame.Batches = v.frame.Batches[:0]
	for _, node := range v.order {
		node.batch = len(v.frame.Batches)
		v.frame.Batches = append(v.frame.Batches, node.batches...)
	}

	v.popupPaint.batch = len(v.frame.Batches)
	v.frame.Batches = append(v.frame.Batches, v.popupPaint.batches...)

	v.tooltip.paint.batch = len(v.frame.Batches)
	if len(v.tooltip.paint.batches) == 0 {
		v.frame.Batches = append(v.frame.Batches, uipaint.Batch{})
	} else {
		v.frame.Batches = append(v.frame.Batches, v.tooltip.paint.batches...)
	}

	if len(v.frame.Batches) < previous {
		clear(v.frame.Batches[len(v.frame.Batches):previous])
	}

	v.batchesDirty = false
}
