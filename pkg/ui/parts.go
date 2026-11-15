package ui

type Part uint8

const (
	Track Part = iota
	Thumb
	Indicator
	LabelPart
	PopupPart
	OptionPart
	TooltipPart
	IconPart
	ImagePart
	ChartGridPart
	ChartAxisPart
	ChartLabelPart
	RipplePart
	ActiveIndicatorPart
	PanelBackgroundPart
)

type State uint16

const (
	Hovered State = 1 << iota
	Pressed
	Focused
	FocusVisible
	Disabled
	Checked
	Mixed
	Selected
	Expanded
	ReadOnly
)

type PartStyle struct {
	Fill, Border Color
	Radius       float32
}
type partRule struct {
	part  Part
	state State
	style PartStyle
}

// SetPartStyle replaces the matching typed rule; later matching rules win.
func (node *Node) SetPartStyle(part Part, state State, style PartStyle) error {
	if part > PanelBackgroundPart || !finite(style.Radius) || style.Radius < 0 {
		return ErrBounds
	}

	for _, color := range []Color{style.Fill, style.Border} {
		for _, child := range color {
			if !finite(child) || child < 0 || child > 1 {
				return ErrBounds
			}
		}
	}

	for index, record := range node.parts {
		if record.part == part && record.state == state {
			node.parts[index].style = style
			node.damage(false)

			return nil
		}
	}

	node.parts = append(node.parts, partRule{part, state, style})
	node.damage(false)

	return nil
}
func (v *View) part(node *Node, part Part, base PartStyle) PartStyle {
	return v.partState(node, part, 0, base)
}

func (v *View) partState(node *Node, part Part, extra State, base PartStyle) PartStyle {
	state := extra
	if v.hover == node {
		state |= Hovered
	}

	if v.pressed == node {
		state |= Pressed
	}

	if v.focus == node {
		state |= Focused
		if v.focusVisible {
			state |= FocusVisible
		}
	}

	if node.disabled {
		state |= Disabled
	}

	if node.checked {
		state |= Checked
	}

	if node.semanticSelected {
		state |= Selected
	}

	if node.mixed {
		state |= Mixed
	}

	if node.readOnly {
		state |= ReadOnly
	}

	if v.popup == node {
		state |= Expanded
	}

	for _, record := range node.parts {
		if record.part == part && state&record.state == record.state {
			base = record.style
		}
	}

	return base
}
