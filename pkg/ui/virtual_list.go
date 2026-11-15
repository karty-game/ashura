package ui

import (
	"errors"
	"math"
)

var errVirtualLayoutRestart = errors.New("ui: restart placement after virtual-row retirement notification")

const (
	MaxVirtualRows    = 1_000_000
	MaxVirtualMounted = 256
	maxVirtualExtent  = 1 << 24
)

// VirtualListOptions describes a fixed-height, vertical collection. Create builds
// one reusable row subtree; Bind updates its existing nodes for a logical index.
// Keep application data outside the rows. Bind must not add/remove nodes, dispatch
// input, or recursively change the list. Errors are reported by View.Frame.
type VirtualListOptions struct {
	Count     int
	RowHeight float32
	Overscan  int
	Create    func() *Node
	Bind      func(*Node, int) error
}

// VirtualListStats exposes the bounded mounted pool and current logical window.
// First/End include overscan; End is exclusive. Bindings is cumulative.
type VirtualListStats struct {
	Count, First, End, Mounted int
	Bindings                   uint64
}

type virtualRow struct {
	node    *Node
	index   int
	refresh bool
	width   float32
}

type virtualList struct {
	options    VirtualListOptions
	rows       []virtualRow
	slots      [MaxVirtualMounted]int
	free       []int
	first, end int
	bindings   uint64
}

// NewVirtualList creates a scrollable Panel; its children are managed internally.
// RowHeight includes spacing. Zero Overscan defaults to two rows on each side.
// A zero Style.Height uses a 240-pixel viewport; Grow can fill available space.
func NewVirtualList(options VirtualListOptions) *Node {
	if options.Overscan == 0 {
		options.Overscan = 2
	}

	node := &Node{kind: Panel, selected: -1, focusable: true, role: "list", style: Style{Scroll: true}}

	node.virtual = &virtualList{options: options}
	if !validVirtualOptions(options) {
		node.constructionErr = ErrBounds
	}

	return node
}

func validVirtualOptions(options VirtualListOptions) bool {
	return options.Count >= 0 && options.Count <= MaxVirtualRows &&
		finite(options.RowHeight) && options.RowHeight >= 1 && options.RowHeight <= 32768 &&
		float64(options.Count)*float64(options.RowHeight) <= maxVirtualExtent &&
		options.Overscan >= 0 && options.Overscan <= 16 && options.Create != nil && options.Bind != nil
}

// SetVirtualCount changes the logical size without traversing application data.
// The next frame clamps scroll position and rebinds only the mounted window.
func (node *Node) SetVirtualCount(count int) error {
	if node.virtual == nil {
		return ErrBounds
	}

	options := node.virtual.options

	options.Count = count
	if !validVirtualOptions(options) {
		return ErrBounds
	}

	if count == node.virtual.options.Count {
		return nil
	}

	node.virtual.options.Count = count

	node.contentHeight = float32(count) * options.RowHeight
	for index := range node.virtual.rows {
		node.virtual.rows[index].refresh = true
	}

	if node.view != nil {
		node.view.stopScroll(node)

		if node.virtualVisible() {
			node.damage(false)
			node.view.placementDirty = true
		} else {
			node.dirty = true
		}
	}

	return nil
}

// RefreshVirtualRows invalidates a logical half-open range. Offscreen updates do
// no rendering work. Work is bounded by the mounted pool, even for a full refresh.
func (node *Node) RefreshVirtualRows(first, count int) error {
	if node.virtual == nil || first < 0 || count < 0 || first > node.virtual.options.Count || count > node.virtual.options.Count-first {
		return ErrBounds
	}

	changed := false

	for index := range node.virtual.rows {
		row := &node.virtual.rows[index]
		if row.index >= first && row.index < first+count {
			row.refresh = true
			changed = true
		}
	}

	if changed && node.virtualVisible() {
		node.view.placementDirty = true
	}

	return nil
}

// ScrollToRow places a logical row at the viewport's start, clamped at the end.
func (node *Node) ScrollToRow(index int) error {
	if node.virtual == nil || index < 0 || index >= node.virtual.options.Count {
		return ErrBounds
	}

	offset := float32(index) * node.virtual.options.RowHeight
	if node.view == nil {
		node.offset = offset
	} else {
		node.view.stopScroll(node)
		// Initial layout has not established a viewport/extent yet.
		if !node.virtualVisible() {
			node.offset = offset
		} else {
			node.view.setScrollOffset(node, offset)
		}
	}

	return nil
}

// Hidden/clipped lists retain pending data changes until a normal placement
// makes them visible. Their fixed viewport size does not depend on item count.
func (node *Node) virtualVisible() bool {
	if node.view == nil || node.hidden || node.layoutRevision != node.view.layoutRevision {
		return false
	}

	visible := node.rect.Intersect(node.clip)

	return visible.W > 0 && visible.H > 0
}

func (node *Node) VirtualListStats() VirtualListStats {
	if node.virtual == nil {
		return VirtualListStats{}
	}

	list := node.virtual

	return VirtualListStats{Count: list.options.Count, First: list.first, End: list.end, Mounted: len(list.rows), Bindings: list.bindings}
}

func (v *View) placeVirtualList(node *Node, inner, clip Rect) error {
	list := node.virtual
	visible := inner.Intersect(clip)
	first, end := 0, 0

	if visible.W > 0 && visible.H > 0 && list.options.Count > 0 {
		height := list.options.RowHeight
		first = max(0, int(math.Floor(float64((visible.Y-inner.Y+node.offset)/height)))-list.options.Overscan)
		end = min(list.options.Count, int(math.Ceil(float64((visible.Y+visible.H-inner.Y+node.offset)/height)))+list.options.Overscan)
	}

	if end-first > MaxVirtualMounted {
		return ErrBounds
	}

	if err := v.growVirtualRows(node, end-first); err != nil {
		return err
	}

	list.first, list.end = first, end

	for index := range list.rows {
		row := &list.rows[index]
		if row.index >= 0 && (row.index < first || row.index >= end) {
			if err := v.retireVirtualRow(node, row.node); err != nil {
				return err
			}

			list.slots[row.index%MaxVirtualMounted] = 0
			row.index = -1
			list.free = append(list.free, index)
		}
	}

	for index := first; index < end; index++ {
		if err := v.placeVirtualRow(node, index, inner, clip); err != nil {
			return err
		}
	}

	return nil
}

func (v *View) placeVirtualRow(node *Node, index int, inner, clip Rect) error {
	list := node.virtual

	slot := index % MaxVirtualMounted
	if list.slots[slot] == 0 {
		last := len(list.free) - 1
		list.slots[slot] = list.free[last] + 1
		list.free = list.free[:last]
	}

	row := &list.rows[list.slots[slot]-1]

	rebound := row.index != index
	if rebound && row.index >= 0 {
		if err := v.retireVirtualRow(node, row.node); err != nil {
			return err
		}

		row.index = -1
	}

	needsMeasure := rebound || row.refresh
	if needsMeasure {
		row.index = index

		row.refresh = true
		if err := list.options.Bind(row.node, index); err != nil {
			return err
		}

		row.index = index
		row.refresh = false
		list.bindings++
	}

	row.node.virtualInactive = false

	row.node.virtualIndex, row.node.virtualSize = index+1, list.options.Count
	if needsMeasure || row.width != inner.W || v.layoutDirty || row.node.dirty {
		if err := v.measure(row.node, inner.W); err != nil {
			return err
		}

		row.width = inner.W
	}

	rowClip := clip.Intersect(
		Rect{X: inner.X, Y: inner.Y + float32(index)*list.options.RowHeight - node.offset, W: inner.W, H: list.options.RowHeight},
	)

	return v.place(
		row.node,
		Rect{
			X: inner.X,
			Y: inner.Y + float32(index)*list.options.RowHeight - node.offset,
			W: min(inner.W, row.node.naturalW),
			H: list.options.RowHeight,
		},
		rowClip,
	)
}

func (v *View) growVirtualRows(node *Node, count int) error {
	list := node.virtual
	if count <= len(list.rows) {
		return nil
	}

	for len(list.rows) < count {
		row := list.options.Create()
		if row == nil {
			return ErrBounds
		}
		// Public Add rejects modifications to managed collections.
		node.virtual = nil
		err := node.Add(row)
		node.virtual = list

		if err != nil {
			return err
		}

		row.virtualInactive = true
		list.rows = append(list.rows, virtualRow{node: row, index: -1})
		list.free = append(list.free, len(list.rows)-1)
	}

	return nil
}

func validateVirtualList(node *Node) error {
	if node.virtual != nil && (node.kind != Panel || !validVirtualOptions(node.virtual.options)) {
		return ErrBounds
	}

	return nil
}
