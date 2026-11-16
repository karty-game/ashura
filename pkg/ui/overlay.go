package ui

// OverlayAnchor selects a viewport corner for a retained nonmodal overlay.
type OverlayAnchor uint8

const (
	OverlayTopRight OverlayAnchor = iota
	OverlayTopLeft
	OverlayBottomRight
	OverlayBottomLeft
)

// OverlayOptions bounds a separate subtree in the existing View. Width zero
// uses the available viewport width (or the root's narrower Style.Width).
// MaxHeightRatio zero selects the full available height; other values are 0..1.
// Margin is logical pixels. Cursor prevents pointer lock while this root is
// visible, without capturing world input outside the overlay's actual bounds.
type OverlayOptions struct {
	Anchor         OverlayAnchor
	Width          float32
	MaxHeightRatio float32
	Margin         float32
	Cursor         bool
}

type overlayEntry struct {
	root    *Node
	options OverlayOptions
}

// AddOverlay admits a detached subtree and returns an idempotent removal
// function. It shares this View's font, theme, resources, input and draw storage.
// Overlays paint above the root in registration order and below modal scopes.
// They do not change normal layout or make unrelated controls modal. All calls,
// including removal, belong to the View's event thread.
func (v *View) AddOverlay(root *Node, options OverlayOptions) (func(), error) {
	if v == nil || root == nil || root.parent != nil || root.view != nil || !validOverlayOptions(options) {
		return nil, ErrBounds
	}

	count := len(v.nodes)
	if err := validateNode(root, 0, &count); err != nil {
		return nil, err
	}

	if v.imageBytes+len(v.font.page.Pixels)+v.additionalImageBytes([]*Node{root}) > MaxImageBytes {
		return nil, ErrBounds
	}

	entry := &overlayEntry{root: root, options: options}
	v.overlays = append(v.overlays, entry)
	v.attach(root)
	root.damage(true)

	return func() { v.removeOverlay(entry) }, nil
}

func validOverlayOptions(options OverlayOptions) bool {
	return options.Anchor <= OverlayBottomLeft &&
		finite(options.Width) && options.Width >= 0 && options.Width <= 32768 &&
		finite(options.Margin) && options.Margin >= 0 && options.Margin <= 32768 &&
		finite(options.MaxHeightRatio) && options.MaxHeightRatio >= 0 && options.MaxHeightRatio <= 1
}

// PreventsPointerLock reports whether a visible overlay requests a usable cursor.
// It does not imply pointer capture or consume input outside that overlay.
func (v *View) PreventsPointerLock() bool {
	for _, entry := range v.overlays {
		if entry.options.Cursor && !entry.root.hidden {
			return true
		}
	}

	return false
}

func (v *View) overlayContains(positionX, positionY float32) bool {
	return v.overlayAt(positionX, positionY) != nil
}

func (v *View) overlayAt(positionX, positionY float32) *Node {
	//nolint:modernize // slices.Backward closures allocate in pinned TinyGo's frame path.
	for index := len(v.overlays) - 1; index >= 0; index-- {
		entry := v.overlays[index]

		root := entry.root
		if !root.hidden && v.inScope(root) && root.rect.Contains(positionX, positionY) && root.clip.Contains(positionX, positionY) {
			return root
		}
	}

	return nil
}

func (v *View) layoutOverlays() error {
	viewport := Rect{W: v.width, H: v.height}
	for _, entry := range v.overlays {
		root := entry.root
		if root.hidden || v.overlayModal(root) {
			continue
		}

		options := entry.options
		margin := min(options.Margin, max(0, (min(v.width, v.height)-1)/2))

		width := max(1, v.width-2*margin)
		if options.Width > 0 {
			width = min(width, options.Width)
		}

		if v.layoutDirty {
			if err := v.measure(root, width); err != nil {
				return err
			}
		}

		width = min(width, root.naturalW)

		ratio := options.MaxHeightRatio
		if ratio == 0 {
			ratio = 1
		}

		height := max(1, min(root.naturalH, (v.height-2*margin)*ratio))

		positionX, positionY := margin, margin
		if options.Anchor == OverlayTopRight || options.Anchor == OverlayBottomRight {
			positionX = v.width - margin - width
		}

		if options.Anchor == OverlayBottomRight || options.Anchor == OverlayBottomLeft {
			positionY = v.height - margin - height
		}

		rect := Rect{X: positionX, Y: positionY, W: width, H: height}
		if err := v.placeNode(root, rect, viewport.Intersect(rect), height < root.naturalH); err != nil {
			return err
		}
	}

	return nil
}

func (v *View) overlayModal(root *Node) bool {
	for _, scope := range v.modals {
		if scope.root == root {
			return true
		}
	}

	return false
}

func (v *View) removeOverlayNode(root *Node) {
	for _, entry := range v.overlays {
		if entry.root == root {
			v.removeOverlay(entry)

			return
		}
	}
}

func (v *View) removeOverlay(entry *overlayEntry) {
	root := entry.root
	if root == nil {
		return
	}

	entry.root = nil
	for index, current := range v.overlays {
		if current == entry {
			copy(v.overlays[index:], v.overlays[index+1:])
			v.overlays[len(v.overlays)-1] = nil
			v.overlays = v.overlays[:len(v.overlays)-1]

			break
		}
	}

	v.clearOverlayInteraction(root)
	v.detach(root)
	v.layoutDirty = true
	v.reconcile()
	v.pruneDetachedOverlayNodes()
}

func (v *View) clearOverlayInteraction(root *Node) {
	if withinNode(v.focus, root) {
		v.SetFocus(nil)
	}

	if v.capture != nil && withinNode(v.capture.node, root) {
		v.cancelCapture()
	}

	if withinNode(v.popup, root) {
		v.closePopup()
	}

	if withinNode(v.hover, root) {
		v.hover = nil
	}

	if withinNode(v.tooltip.node, root) {
		v.dismissTooltip()
		v.tooltip.node = nil
	}

	v.resumeTooltip()
}

func (v *View) pruneDetachedOverlayNodes() {
	v.order = keepAttached(v.order, v)
	v.dirty = keepAttached(v.dirty, v)
	v.controlMotions = keepAttached(v.controlMotions, v)

	kept := v.scrolls[:0]
	for _, motion := range v.scrolls {
		if motion.node.view == v {
			kept = append(kept, motion)
		}
	}

	clear(v.scrolls[len(kept):])
	v.scrolls = kept
}

func keepAttached(nodes []*Node, view *View) []*Node {
	kept := nodes[:0]
	for _, node := range nodes {
		if node.view == view {
			kept = append(kept, node)
		}
	}

	clear(nodes[len(kept):])

	return kept
}
