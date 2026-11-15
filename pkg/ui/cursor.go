package ui

import "github.com/karty-game/karty-webgpu/internal/uihost"

// Cursor is the platform cursor requested by a UI surface.
// Wrappers apply it to their canvas or native window, without rendering a cursor in the UI.
type Cursor = uihost.Cursor

const (
	DefaultCursor          = uihost.DefaultCursor
	PointerCursor          = uihost.PointerCursor
	TextCursor             = uihost.TextCursor
	HorizontalResizeCursor = uihost.HorizontalResizeCursor
	VerticalResizeCursor   = uihost.VerticalResizeCursor
)

// CursorAt resolves the cursor at logical surface coordinates. Mouse drags retain
// their manipulation cursor until capture ends, even outside the original control.
// Call after Frame when layout changes, and when the mouse moves.
func (v *View) CursorAt(positionX, positionY float32) Cursor {
	if !finite(positionX) || !finite(positionY) {
		return DefaultCursor
	}

	if child := v.capture; child != nil && child.device != deviceTouch && v.validCapture() {
		if child.scrollbar {
			return VerticalResizeCursor
		}

		if child.popup {
			return PointerCursor
		}

		return nodeCursor(child.node)
	}

	if v.popup != nil && v.popupRect.Contains(positionX, positionY) {
		index := int((positionY - v.popupRect.Y + v.popupOffset) / v.theme.ControlHeight)
		if index >= 0 && index < len(v.popup.options) && !v.popup.options[index].Disabled {
			return PointerCursor
		}

		return DefaultCursor
	}

	if node := v.scrollAt(positionX, positionY); node != nil && scrollExtent(node) > 0 {
		track, _ := scrollbarGeometry(node)
		track.X -= 3
		track.W += 6

		if track.Intersect(node.clip).Contains(positionX, positionY) {
			return VerticalResizeCursor
		}
	}

	return nodeCursor(v.hit(positionX, positionY))
}

func nodeCursor(node *Node) Cursor {
	if node == nil || !node.eligible() {
		return DefaultCursor
	}

	if node.kind == Input {
		return TextCursor
	}

	if node.readOnly {
		return DefaultCursor
	}

	if node.kind == Slider {
		return HorizontalResizeCursor
	}

	if isControl(node.kind) {
		return PointerCursor
	}

	return DefaultCursor
}
