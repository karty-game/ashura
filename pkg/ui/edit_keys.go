package ui

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

func (v *View) editKey(node *Node, event Event) {
	if !v.refreshEditorLayout(node) {
		return
	}

	if event.Modifiers.Control || event.Modifiers.Meta {
		v.pending.Consumed = v.editCommand(node, event)

		return
	}

	switch event.Key {
	case keyLeft, keyRight:
		v.moveCursor(node, event, false)
	case keyHome:
		v.editBoundary(node, event, 0)
	case keyEnd:
		v.editBoundary(node, event, len(node.text))
	case "Backspace", "Delete":
		v.deleteEdit(node, event.Key == "Backspace")
	case keyEnter:
		v.finish(node)
	default:
		return
	}

	v.pending.Consumed = true
	v.caretVisible = true
}
func (v *View) editCommand(node *Node, event Event) bool {
	switch strings.ToLower(event.Key) {
	case "a":
		node.anchor, node.cursor = 0, len(node.text)
		node.damage(false)
	case "c", "x":
		v.pending.Copy = node.text[min(node.anchor, node.cursor):max(node.anchor, node.cursor)]

		v.pending.ClipboardWrite = true
		if strings.EqualFold(event.Key, "x") && !node.readOnly {
			v.editEvent(Event{Kind: InsertText})
		}
	case "v":
		v.pending.ClipboardRead = true
		v.pending.Session = v.editSession
		v.pending.Revision = node.revision
	case "z":
		v.restoreEdit(node, event.Modifiers.Shift)
	case "y":
		v.restoreEdit(node, true)
	case "arrowleft", "arrowright":
		v.moveCursor(node, event, true)
	case "home":
		v.editBoundary(node, event, 0)
	case "end":
		v.editBoundary(node, event, len(node.text))
	default:
		return false
	}

	return true
}
func (v *View) editBoundary(node *Node, event Event, offset int) {
	node.cursor = offset
	if !event.Modifiers.Shift {
		node.anchor = offset
	}

	node.damage(false)
}
func (v *View) deleteEdit(node *Node, backward bool) {
	if node.readOnly {
		return
	}

	before := snapshot{node.text, node.anchor, node.cursor}
	if node.anchor == node.cursor {
		previous, next := 0, len(node.text)
		for _, bound := range graphemes(node.text) {
			if bound < node.cursor {
				previous = bound
			}

			if bound > node.cursor {
				next = bound

				break
			}
		}

		if backward {
			node.anchor = previous
		} else {
			node.anchor = next
		}
	}

	v.editEvent(Event{Kind: InsertText})

	if node.text != before.text && len(node.undo) > 0 {
		node.undo[len(node.undo)-1] = before
	}
}
func (v *View) moveCursor(node *Node, event Event, word bool) {
	left := event.Key == keyLeft
	switch {
	case !event.Modifiers.Shift && node.anchor != node.cursor && !word:
		if left {
			node.cursor = min(node.anchor, node.cursor)
		} else {
			node.cursor = max(node.anchor, node.cursor)
		}
	case word:
		node.cursor = wordPosition(node.text, node.cursor, left)
	default:
		node.cursor = visualPosition(node, left)
	}

	if !event.Modifiers.Shift {
		node.anchor = node.cursor
	}

	node.damage(false)
}
func wordPosition(text string, offset int, left bool) int {
	if left {
		for offset > 0 {
			runeValue, size := utf8.DecodeLastRuneInString(text[:offset])
			if !unicode.IsSpace(runeValue) {
				break
			}

			offset -= size
		}

		for offset > 0 {
			runeValue, size := utf8.DecodeLastRuneInString(text[:offset])
			if unicode.IsSpace(runeValue) {
				break
			}

			offset -= size
		}

		return offset
	}

	for offset < len(text) {
		runeValue, size := utf8.DecodeRuneInString(text[offset:])
		if unicode.IsSpace(runeValue) {
			break
		}

		offset += size
	}

	for offset < len(text) {
		runeValue, size := utf8.DecodeRuneInString(text[offset:])
		if !unicode.IsSpace(runeValue) {
			break
		}

		offset += size
	}

	return offset
}
func visualPosition(node *Node, left bool) int {
	positionX, positionY := node.layout.position(node.cursor)
	best := float32(1e20)

	offset := node.cursor
	for _, bound := range graphemes(node.text) {
		nextX, nextY := node.layout.position(bound)

		delta := nextX - positionX
		if left {
			delta = -delta
		}

		if nextY == positionY && delta > 0.01 && delta < best {
			best = delta
			offset = bound
		}
	}

	return offset
}
