//nolint:mnd // UI metrics, coordinate equations and bounded prepared-format widths are explicit.
package ui

import (
	"time"

	"github.com/karty-game/ashura/internal/uihost"
	"github.com/karty-game/ashura/internal/uipaint"
)

type capture struct {
	scrollbar                          bool
	scrollStart                        float32
	node, scroller                     *Node
	pointer                            int64
	positionX, positionY, lastX, lastY float32
	dragging, scrolling, popup         bool
	option                             int
	device                             string
	startValue                         float64
}
type modal struct {
	root, restore         *Node
	wasHidden, wasOverlay bool
	backdrop              Node
}

// View owns one retained UI surface. All methods belong to its platform/event thread.
type View struct {
	overlays                     []*overlayEntry
	previews                     []*viewPreview
	previewsDirty                bool
	virtualNotifications         [2]virtualNotification
	virtualNotificationCount     int
	controlMotions               []*Node
	images                       map[*uipaint.Texture]int
	batchesDirty                 bool
	layoutRevision               uint64
	now                          time.Duration
	placementDirty               bool
	scrolls                      []scrollMotion
	reducedMotion                bool
	tooltip                      tooltipState
	popupNeedsReveal             bool
	imageBytes                   int
	dispatching                  bool
	free                         []recordBlock
	editSession                  ID
	root                         *Node
	font                         *Font
	theme                        Theme
	nodes                        map[ID]*Node
	next                         ID
	width, height                float32
	layoutDirty                  bool
	dirty                        []*Node
	order                        []*Node
	focus, hover, pressed, popup *Node
	popupRect                    Rect
	popupOffset                  float32
	popupPaint                   Node
	capture                      *capture
	keyPressed                   string
	focusVisible                 bool
	modals                       []modal
	frame                        uipaint.Frame
	used                         int
	capabilities                 Capabilities
	lastBlink                    int64
	caretVisible                 bool
	typeahead                    string
	typeaheadAt                  time.Duration
	pending                      Result
	stats                        Stats
}

type Stats = uihost.Stats

func New(root *Node, font *Font) (*View, error) {
	count := 0

	if root == nil || font == nil || root.parent != nil || root.view != nil {
		return nil, ErrBounds
	}

	if err := validateNode(root, 0, &count); err != nil {
		return nil, err
	}

	if preparedImageBytes(root)+len(font.page.Pixels) > MaxImageBytes {
		return nil, ErrBounds
	}

	v := &View{root: root, font: font, theme: DarkTheme(), nodes: make(map[ID]*Node), layoutDirty: true, caretVisible: true}
	v.attach(root)

	return v, nil
}

func (v *View) Root() *Node        { return v.root }
func (v *View) Focused() *Node     { return v.focus }
func (v *View) Lookup(id ID) *Node { return v.nodes[id] }

// FocusedID returns the focused control's identity, or zero when none is focused.
func (v *View) FocusedID() ID {
	if v.focus == nil {
		return 0
	}

	return v.focus.id
}

func (v *View) Stats() Stats {
	state := v.stats
	state.Nodes = len(v.nodes)
	state.Records = v.used

	return state
}
func (v *View) SetCapabilities(child Capabilities) { v.capabilities = child }
func (v *View) Capabilities() Capabilities         { return v.capabilities }
func (v *View) SetTheme(theme Theme) error {
	if err := validateTheme(theme); err != nil {
		return err
	}

	v.theme = theme

	v.layoutDirty = true
	for _, node := range v.nodes {
		node.damage(false)
	}

	return nil
}
func (v *View) Theme() Theme { return v.theme }
func (v *View) Resize(width, height float32) error {
	if !finite(width) || !finite(height) || width <= 0 || height <= 0 || width > 32768 || height > 32768 {
		return ErrBounds
	}

	if v.width != width || v.height != height {
		v.width, v.height = width, height
		v.layoutDirty = true
	}

	return nil
}
func (v *View) SetFocus(node *Node) {
	if node != nil && (!node.eligible() || node.view != v || !v.inScope(node)) {
		return
	}

	if v.focus == node {
		return
	}

	if v.focus != nil {
		old := v.focus
		v.cancelControlRipple(old)

		if v.capture == nil && v.pressed == old {
			v.pressed = nil
		}

		if old.composing {
			v.cancelComposition(old)
		}

		old.damage(false)
	}

	v.focus = node
	v.resumeTooltip()
	v.editSession++
	v.keyPressed = ""
	v.caretVisible = true

	if node != nil {
		node.damage(false)
		v.reveal(node)
	}
}

func (v *View) PushModal(root *Node) error {
	if root == nil || root.view != v {
		return ErrBounds
	}

	for _, entry := range v.modals {
		if entry.root == root {
			return ErrBounds
		}
	}

	v.modals = append(v.modals, modal{root: root, restore: v.focus, wasHidden: root.hidden, wasOverlay: root.overlay})
	root.hidden = false
	root.overlay = true

	if err := v.layoutWithVirtualRetirement(); err != nil {
		return err
	}

	v.cancelCapture()
	v.closePopup()
	v.SetFocus(nil)
	v.tab(false)
	v.layoutDirty = true

	return nil
}
func (v *View) PopModal() {
	if len(v.modals) == 0 {
		return
	}

	entry := v.modals[len(v.modals)-1]

	entry.root.hidden, entry.root.overlay = entry.wasHidden, entry.wasOverlay
	if entry.backdrop.capacity > 0 {
		v.free = append(v.free, recordBlock{entry.backdrop.slot, entry.backdrop.capacity})
	}

	v.modals[len(v.modals)-1] = modal{}
	v.modals = v.modals[:len(v.modals)-1]
	v.closePopup()
	v.cancelCapture()
	v.SetFocus(nil)
	v.SetFocus(entry.restore)

	if v.focus == nil {
		v.tab(false)
	}

	v.layoutDirty = true
}

func (v *View) CapturesKeyboard() bool {
	return len(v.modals) > 0 || v.popup != nil || (v.focus != nil && v.focus.kind == Input)
}
func (v *View) CapturesPointer() bool { return v.capture != nil || len(v.modals) > 0 }
func (v *View) ConsumesPointer(positionX, positionY float32) bool {
	return v.CapturesPointer() || v.hit(positionX, positionY) != nil || v.scrollAt(positionX, positionY) != nil ||
		v.popup != nil && v.popupRect.Contains(positionX, positionY) || v.overlayContains(positionX, positionY)
}

// CapturedPointer identifies the current press/drag owner for wrapper reconciliation.
func (v *View) CapturedPointer() (int64, bool) {
	if v.capture == nil {
		return 0, false
	}

	return v.capture.pointer, true
}

// Frame returns borrowed persistent draw storage. Static frames reuse its revision.
func (v *View) Frame(now time.Duration) (*uipaint.Frame, error) {
	v.now = now
	v.advanceControlMotion(now)
	v.advanceScroll(now)
	v.updateTooltip(now)

	if v.width == 0 || v.height == 0 {
		return nil, ErrBounds
	}

	blink := now.Milliseconds() / 530
	if v.focus != nil && v.focus.kind == Input && blink != v.lastBlink {
		v.lastBlink = blink
		v.caretVisible = blink%2 == 0
		v.focus.damage(false)
	}

	if !v.layoutDirty && !v.placementDirty && len(v.dirty) == 0 && !v.tooltip.dirty {
		return &v.frame, nil
	}

	v.frame.Base = v.frame.Revision
	v.frame.Revision++
	v.frame.Width, v.frame.Height = v.width, v.height

	v.frame.Changes = v.frame.Changes[:0]
	if v.layoutDirty || v.placementDirty {
		if err := v.rebuildFrame(); err != nil {
			return nil, err
		}
	} else if err := v.repaintDirty(); err != nil {
		return nil, err
	}

	if err := v.paintPopup(); err != nil {
		return nil, err
	}

	if err := v.paintTooltip(); err != nil {
		return nil, err
	}

	if v.batchesDirty {
		v.rebuildBatches()
	}

	v.frame.Data = v.frame.Data[:v.used*uipaint.Stride]

	return &v.frame, nil
}

func (v *View) rebuildFrame() error {
	v.batchesDirty = true

	if err := v.layoutWithVirtualRetirement(); err != nil {
		return err
	}

	for _, node := range v.order {
		if node.dirty || node.capacity == 0 {
			if err := v.paintNode(node); err != nil {
				return err
			}
		}

		node.dirty = false
	}

	v.layoutDirty = false
	v.placementDirty = false
	v.dirty = v.dirty[:0]

	return nil
}

func (v *View) repaintDirty() error {
	for _, node := range v.dirty {
		if node.view == v && !node.hidden && node.layoutRevision == v.layoutRevision {
			if err := v.paintNode(node); err != nil {
				return err
			}

			node.dirty = false
		}
	}

	v.dirty = v.dirty[:0]

	return nil
}
