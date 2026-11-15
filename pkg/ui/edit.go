//nolint:mnd // UI metrics, coordinate equations and bounded prepared-format widths are explicit.
package ui

import (
	"strings"
	"unicode/utf8"

	"github.com/go-text/typesetting/segmenter"
)

func boundary(text string, index int) bool {
	return index >= 0 && index <= len(text) && (index == len(text) || utf8.RuneStart(text[index]))
}
func graphemes(text string) []int {
	runes := []rune(text)

	positions := make([]int, 0, len(runes)+1)
	for index := range text {
		positions = append(positions, index)
	}

	positions = append(positions, len(text))

	var state segmenter.Segmenter
	state.Init(runes)
	it := state.GraphemeIterator()
	result := []int{0}

	for it.Next() {
		glyph := it.Grapheme()
		result = append(result, positions[glyph.Offset+len(glyph.Text)])
	}

	return result
}
func (v *View) editEvent(event Event) {
	node := v.focus
	if node == nil || node.kind != Input || !node.eligible() {
		return
	}

	v.pending.Consumed = true

	if event.Session != 0 && event.Session != v.editSession || event.Revision != 0 && event.Revision != node.revision {
		return
	}

	//nolint:exhaustive // Each controller handles its declared subset; other event/control kinds are ignored or use the default.
	switch event.Kind {
	case SetSelection:
		if boundary(node.text, event.Anchor) && boundary(node.text, event.Cursor) {
			node.anchor, node.cursor = event.Anchor, event.Cursor
			node.damage(false)
		}
	case CompositionStart:
		if node.readOnly || node.composing {
			return
		}

		node.composing = true
		node.composition = snapshot{node.text, node.anchor, node.cursor}
		node.compositionStart, node.compositionEnd = min(node.anchor, node.cursor), max(node.anchor, node.cursor)
		node.damage(false)
	case CompositionEnd:
		if !node.composing {
			return
		}

		node.composing = false
		if node.text != node.composition.text {
			node.pushUndo(node.composition)
		}

		node.damage(false)
		node.changeValue()
	case CompositionCancel:
		v.cancelComposition(node)
	case InsertText, ReplaceText:
		v.replaceEdit(node, event)
	}
}
func (node *Node) pushUndo(state snapshot) {
	if len(node.undo) >= 64 {
		copy(node.undo, node.undo[1:])
		node.undo = node.undo[:63]
	}

	node.undo = append(node.undo, state)
	node.redo = node.redo[:0]
}
func (v *View) cancelComposition(node *Node) {
	if !node.composing {
		return
	}

	state := node.composition
	node.text, node.anchor, node.cursor = state.text, state.anchor, state.cursor
	node.composing = false
	node.revision++
	node.damage(false)
	node.changeValue()
}

func (v *View) restoreEdit(node *Node, redo bool) {
	if node.readOnly {
		return
	}

	from, destination := &node.undo, &node.redo
	if redo {
		from, destination = &node.redo, &node.undo
	}

	if len(*from) == 0 {
		return
	}

	state := (*from)[len(*from)-1]
	*from = (*from)[:len(*from)-1]

	*destination = append(*destination, snapshot{node.text, node.anchor, node.cursor})
	node.text, node.anchor, node.cursor = state.text, state.anchor, state.cursor
	node.revision++
	node.damage(false)
	node.changeValue()
}
func (v *View) Editing() EditingState {
	node := v.focus
	if node == nil || node.kind != Input || !node.eligible() || node.readOnly {
		return EditingState{}
	}

	if node.layout.text != node.text {
		if layout, err := v.shape(node.text, v.fontSize(node), 32768, false); err == nil {
			node.layout = layout
		}
	}

	positionX, positionY := node.layout.position(node.cursor)

	return EditingState{
		Active:   true,
		Session:  v.editSession,
		Revision: node.revision,
		Text:     node.text,
		Anchor:   node.anchor,
		Cursor:   node.cursor,
		Bounds:   node.rect,
		Caret: Rect{
			X: node.rect.X + 12 + positionX - node.editOffset,
			Y: node.rect.Y + (node.rect.H-node.layout.height)/2 + positionY,
			W: 1,
			H: v.fontSize(node) * 1.4,
		},
		Composing: node.composing,
	}
}

// CharacterBounds maps a UTF-8 text position to logical surface geometry for IME services.
func (v *View) CharacterBounds(offset int) Rect {
	node := v.focus
	if node == nil || node.kind != Input || !boundary(node.text, offset) {
		return Rect{}
	}

	positionX, positionY := node.layout.position(offset)

	next := offset
	if next < len(node.text) {
		_, size := utf8.DecodeRuneInString(node.text[next:])
		next += size
	}

	end, _ := node.layout.position(next)

	return Rect{
		X: node.rect.X + 12 + min(positionX, end) - node.editOffset,
		Y: node.rect.Y + (node.rect.H-node.layout.height)/2 + positionY,
		W: max(1, abs(end-positionX)),
		H: v.fontSize(node) * 1.4,
	}
}

func (v *View) replaceEdit(node *Node, event Event) {
	if node.readOnly || len(event.Text) > MaxTextBytes || !utf8.ValidString(event.Text) {
		return
	}

	start, end := min(node.anchor, node.cursor), max(node.anchor, node.cursor)
	if event.Kind == ReplaceText {
		start, end = event.Start, event.End
	}

	if !boundary(node.text, start) || !boundary(node.text, end) || start > end {
		return
	}

	text := strings.Map(func(record rune) rune {
		if record == '\r' || record == '\n' || record == 0 {
			return -1
		}

		return record
	}, event.Text)
	if len(node.text)-end+start+len(text) > MaxTextBytes {
		return
	}

	if !node.composing {
		node.pushUndo(snapshot{node.text, node.anchor, node.cursor})
	}

	node.text = node.text[:start] + text + node.text[end:]
	node.cursor = start + len(text)
	node.anchor = node.cursor

	node.revision++
	if event.Kind == ReplaceText && boundary(node.text, event.Anchor) && boundary(node.text, event.Cursor) {
		node.anchor, node.cursor = event.Anchor, event.Cursor
	}

	node.damage(false)

	v.caretVisible = true

	node.changeValue()
}
