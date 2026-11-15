package ui

// ButtonVariant identifies an action's visual priority independently of its label.
type ButtonVariant uint8

const (
	SecondaryButton ButtonVariant = iota
	PrimaryButton
	QuietButton
)

const (
	quietHoverOpacity = 0.04
	quietPressOpacity = 0.07
)

// ButtonPalette keeps each action's foreground readable through pointer states.
type ButtonPalette struct {
	Fill, Hover, Pressed, Foreground, Border Color
}

// SetButtonVariant changes presentation without changing activation or focus.
func (node *Node) SetButtonVariant(variant ButtonVariant) error {
	if (node.kind != Button && node.kind != ToggleButton) || variant > QuietButton {
		return ErrBounds
	}

	if node.buttonVariant != variant {
		node.buttonVariant = variant
		node.damage(false)
	}

	return nil
}

func (node *Node) ButtonVariant() ButtonVariant { return node.buttonVariant }

func (v *View) controlPaint(node *Node, fill, foreground, border Color) (Color, Color, Color) {
	if node.kind == Button || node.kind == ToggleButton {
		fill, foreground, border = v.buttonPaint(node)
	} else {
		if fill == (Color{}) {
			fill = v.theme.Control
		}

		if v.hover == node {
			fill = v.theme.Hover
		}

		if v.pressed == node {
			fill = v.theme.Pressed
		}
	}

	if node.style.Fill != (Color{}) {
		fill = node.style.Fill
	}

	if node.style.Foreground != (Color{}) {
		foreground = node.style.Foreground
	}

	if node.style.Border != (Color{}) {
		border = node.style.Border
	}

	if inheritedDisabled(node) {
		fill = v.theme.Surface
		foreground = v.theme.Muted
		border = v.theme.Border
	}

	return fill, foreground, border
}

func (v *View) buttonPaint(node *Node) (Color, Color, Color) {
	palette := v.theme.SecondaryButton
	switch node.buttonVariant {
	case PrimaryButton:
		palette = v.theme.PrimaryButton
	case QuietButton:
		hover, pressed := v.theme.Foreground, v.theme.Foreground
		hover[3] *= quietHoverOpacity
		pressed[3] *= quietPressOpacity
		palette = ButtonPalette{Hover: hover, Pressed: pressed, Foreground: v.theme.Foreground}
	case SecondaryButton:
	}

	if node.semanticSelected || node.checked {
		palette = v.theme.PrimaryButton
	}

	fill := palette.Fill
	if v.hover == node {
		fill = palette.Hover
	}

	if v.pressed == node {
		fill = palette.Pressed
	}

	return fill, palette.Foreground, palette.Border
}
