//nolint:mnd // Form spacing and preferred field width are theme-scale layout defaults.
package ui

import (
	"strings"
	"unicode/utf8"
)

type fieldState struct {
	control, label, help, error *Node
}

// NewField composes a visible label, a detached value control and retained help/error text.
// Fields grow in rows with a preferred width; a wrapping row can shrink them on small screens.
// Controls with inline button/boolean labels retain those names; the field is their named group.
func NewField(label string, control *Node) *Node {
	root := Column().WithStyle(Style{Gap: 6, Grow: 1, Basis: 240})
	if !validFieldLabel(label) || !validFieldControl(control) {
		root.constructionErr = ErrBounds

		return root
	}

	caption := NewText(label).WithStyle(Style{FontSize: 13})
	help := NewText("").WithStyle(Style{FontSize: 13})
	help.fieldHint, help.hidden = true, true
	errorText := NewText("").WithStyle(Style{FontSize: 13})
	errorText.fieldError, errorText.hidden = true, true
	root.field = &fieldState{control: control, label: caption, help: help, error: errorText}
	root.role, root.label = "group", label

	root.constructionErr = root.Add(caption, control, help, errorText)
	if root.constructionErr != nil {
		return root
	}

	if !inlineFieldControl(control.kind) {
		root.constructionErr = control.SetLabel(label, control.description)
		control.labelledBy = caption
	}

	control.describedBy, control.invalid = nil, false

	return root
}

// NewForm lays out content vertically. Use wrapping Rows for responsive groups of fields.
// Submission remains an explicit application callback rather than a platform form action.
func NewForm(content ...*Node) *Node {
	node := Column().WithStyle(Style{Gap: 16})
	node.constructionErr = node.Add(content...)

	return node
}

// SetHelpText updates a field's hint without changing its control's value or focus.
// Empty text hides the hint. An active error takes precedence in the description relation.
func (node *Node) SetHelpText(text string) error {
	if !node.validField() {
		return ErrBounds
	}

	if err := node.field.help.SetText(text); err != nil {
		return err
	}

	node.field.help.SetHidden(text == "")
	node.describeField()

	return nil
}

// SetErrorText marks the field control invalid and exposes the error as its description.
// Clearing the error restores the hint relation. This is an application update, not a change event.
func (node *Node) SetErrorText(text string) error {
	if !node.validField() {
		return ErrBounds
	}

	if err := node.field.error.SetText(text); err != nil {
		return err
	}

	node.field.error.SetHidden(text == "")
	control := node.field.control

	invalid := text != ""
	if control.invalid != invalid {
		control.invalid = invalid
		control.damage(false)
	}

	node.describeField()

	return nil
}

func (node *Node) validField() bool {
	if node.field == nil {
		return false
	}

	state := node.field

	return state.control.parent == node && state.label.parent == node && state.help.parent == node && state.error.parent == node
}

func (node *Node) describeField() {
	state := node.field

	state.control.describedBy = nil
	if !state.error.hidden {
		state.control.describedBy = state.error
	} else if !state.help.hidden {
		state.control.describedBy = state.help
	}
}

func validFieldLabel(label string) bool {
	return len(label) <= MaxTextBytes && utf8.ValidString(label) && strings.TrimSpace(label) != ""
}

func validFieldControl(control *Node) bool {
	if control == nil || control.parent != nil || control.view != nil || (!isControl(control.kind) && control.kind != Progress) {
		return false
	}

	count := 4

	return validateNode(control, 1, &count) == nil
}

func inlineFieldControl(kind Kind) bool {
	return kind == Checkbox || kind == Switch || kind == Button || kind == ToggleButton
}
