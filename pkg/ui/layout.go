//nolint:mnd // UI metrics, coordinate equations and bounded prepared-format widths are explicit.
package ui

func (v *View) layout() error {
	v.layoutRevision++
	v.stats.Layouts++

	v.order = v.order[:0]
	if v.layoutDirty {
		v.stats.Measures++
		if err := v.measure(v.root, v.width); err != nil {
			return err
		}
	}

	if err := v.place(v.root, Rect{W: v.width, H: v.height}, Rect{W: v.width, H: v.height}); err != nil {
		return err
	}

	if err := v.layoutOverlays(); err != nil {
		return err
	}

	v.reconcileModals()

	for index := range v.modals {
		entry := &v.modals[index]
		root := entry.root

		width := max(1, min(v.width-32, float32(420)))
		if root.style.Width > 0 {
			width = min(width, root.style.Width)
		}

		if v.layoutDirty {
			if err := v.measure(root, width); err != nil {
				return err
			}
		}

		height := max(1, min(v.height-32, root.naturalH))
		backdrop := &entry.backdrop
		backdrop.view = v
		backdrop.layoutRevision = v.layoutRevision
		backdrop.kind = Panel
		backdrop.style.Fill = Color{0, 0, 0, 0.65}

		rect := Rect{W: v.width, H: v.height}
		if backdrop.rect != rect {
			backdrop.rect = rect
			backdrop.clip = rect
			backdrop.damage(false)
		}

		v.order = append(v.order, backdrop)
		if err := v.place(root, Rect{X: (v.width - width) / 2, Y: (v.height - height) / 2, W: width, H: height}, rect); err != nil {
			return err
		}
	}

	return nil
}

func (v *View) fontSize(node *Node) float32 {
	if node.style.FontSize > 0 {
		return node.style.FontSize
	}

	return v.theme.FontSize
}
func (v *View) gap(node *Node) float32 {
	if node.style.Gap > 0 {
		return node.style.Gap
	}

	return v.theme.Gap
}

func (v *View) measure(node *Node, width float32) error {
	if node.hidden {
		node.naturalW, node.naturalH = 0, 0

		return nil
	}

	state := node.style
	if state.Width > 0 {
		width = min(width, state.Width)
	}

	if state.MaxWidth > 0 {
		width = min(width, state.MaxWidth)
	}

	width = max(width, state.MinWidth)
	inner := max(1, width-2*state.Padding)

	node.naturalW = width
	//nolint:exhaustive // Each controller handles its declared subset; other event/control kinds are ignored or use the default.
	switch node.kind {
	case Panel:
		if err := v.measurePanel(node, inner); err != nil {
			return err
		}
	case Text:
		layout, err := v.shape(node.text, v.fontSize(node), inner, true)
		if err != nil {
			return err
		}

		node.layout = layout
		node.naturalH = layout.height + 2*state.Padding
	case Separator:
		node.naturalH = 1 + 2*state.Padding
	case Progress:
		node.naturalH = v.theme.ControlHeight + 2*state.Padding
	case Image:
		node.naturalH = 64 + 2*state.Padding
	case Chart:
		node.naturalH = 240 + 2*state.Padding
	default:
		if err := v.measureControl(node); err != nil {
			return err
		}
	}

	if state.Height > 0 {
		node.naturalH = state.Height
	}

	return nil
}

func (v *View) measureControl(node *Node) error {
	node.naturalH = v.theme.ControlHeight + 2*node.style.Padding
	if node.kind == Button || node.kind == ToggleButton {
		node.naturalH = max(node.naturalH, v.measureButtonVisual(node))
	}

	text := node.text
	if node.kind == Checkbox || node.kind == Switch {
		text = node.label
	}

	if node.kind == Select && node.selected >= 0 {
		text = node.options[node.selected].Label
	}

	if node.layout.text != text || node.layout.size != v.fontSize(node) {
		var err error

		node.layout, err = v.shape(text, v.fontSize(node), 32768, false)
		if err != nil {
			return err
		}
	}

	return nil
}

func (v *View) place(node *Node, rect, clip Rect) error {
	return v.placeNode(node, rect, clip, false)
}

func (v *View) placeNode(node *Node, rect, clip Rect, fitGrow bool) error {
	if node.hidden {
		return nil
	}

	if node.rect != rect || node.clip != clip {
		node.damage(false)
	}

	node.rect, node.clip = rect, clip
	node.layoutRevision = v.layoutRevision

	v.order = append(v.order, node)
	if node.kind != Panel {
		return nil
	}

	inner := rect.Inset(node.style.Padding)
	if node.style.Scroll {
		clip = clip.Intersect(inner)
		node.offset = max(0, min(max(0, node.contentHeight-inner.H), node.offset))
	}

	if node.virtual != nil {
		return v.placeVirtualList(node, inner, clip)
	}

	if node.style.Direction == Horizontal {
		return v.placeRow(node, inner, clip)
	}

	axis := inner.Y - node.offset
	total, grow, count := flexTotals(node)
	total += float32(max(0, count-1)) * v.gap(node)
	space := inner.H - total

	for _, child := range node.children {
		if child.hidden || child.overlay {
			continue
		}

		width, height := min(inner.W, child.naturalW), child.naturalH
		if child.style.Height == 0 && child.style.Grow > 0 && grow > 0 && (space > 0 || fitGrow) {
			height = max(0, height+space*child.style.Grow/grow)
		}

		if err := v.place(child, Rect{X: inner.X, Y: axis, W: max(0, width), H: max(0, height)}, clip); err != nil {
			return err
		}

		axis += height + v.gap(node)
	}

	return nil
}

func (v *View) hit(positionX, positionY float32) *Node {
	boundary := v.overlayAt(positionX, positionY)
	// Reverse indexes preserve hit order without TinyGo iterator closures.
	//nolint:modernize // slices.Backward closures allocate in pinned TinyGo's frame path.
	for index := len(v.order) - 1; index >= 0; index-- {
		node := v.order[index]
		if node.eligible() && v.inScope(node) && node.rect.Contains(positionX, positionY) && node.clip.Contains(positionX, positionY) {
			if boundary == nil || withinNode(node, boundary) {
				return node
			}
		}

		if node == boundary {
			return nil
		}
	}

	return nil
}
func (v *View) scrollAt(positionX, positionY float32) *Node {
	boundary := v.overlayAt(positionX, positionY)
	//nolint:modernize // slices.Backward closures allocate in pinned TinyGo's frame path.
	for index := len(v.order) - 1; index >= 0; index-- {
		node := v.order[index]
		if node.style.Scroll && modalAvailable(node, v) && node.rect.Contains(positionX, positionY) &&
			node.clip.Contains(positionX, positionY) &&
			v.inScope(node) {
			if boundary == nil || withinNode(node, boundary) {
				return node
			}
		}

		if node == boundary {
			return nil
		}
	}

	return nil
}

func (v *View) measurePanel(node *Node, inner float32) error {
	if node.virtual != nil {
		node.contentHeight = float32(node.virtual.options.Count) * node.virtual.options.RowHeight
		node.naturalH = 240 + 2*node.style.Padding

		return nil
	}

	if node.style.Direction == Horizontal {
		return v.measureRow(node, inner)
	}

	total, count := float32(0), 0

	for _, child := range node.children {
		if child.hidden || child.overlay {
			continue
		}

		if err := v.measure(child, inner); err != nil {
			return err
		}

		total += child.naturalH
		count++
	}

	total += float32(max(0, count-1)) * v.gap(node)
	node.contentHeight = total
	node.naturalH = total + 2*node.style.Padding

	return nil
}

func flexTotals(node *Node) (total, grow float32, count int) {
	for _, child := range node.children {
		if child.hidden || child.overlay {
			continue
		}

		total += child.naturalH
		if child.style.Height == 0 {
			grow += child.style.Grow
		}

		count++
	}

	return total, grow, count
}
