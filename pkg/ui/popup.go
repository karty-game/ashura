//nolint:mnd // UI metrics, coordinate equations and bounded prepared-format widths are explicit.
package ui

func (v *View) paintPopup() error {
	popupState := &v.popupPaint

	popupState.paint = popupState.paint[:0]
	if v.popup == nil {
		return v.writeBlock(popupState, v.font.page)
	}

	node := v.popup
	height := min(float32(len(node.options))*v.theme.ControlHeight, min(260, v.height-16))

	positionY := node.rect.Y + node.rect.H + 5
	if positionY+height > v.height-8 {
		positionY = max(8, node.rect.Y-height-5)
	}

	v.popupRect = Rect{X: node.rect.X, Y: positionY, W: min(node.rect.W, v.width-node.rect.X-8), H: height}
	clip := v.popupRect

	v.popupOffset = max(0, min(v.popupExtent(), v.popupOffset))
	if v.popupNeedsReveal {
		v.revealOption()
		v.popupNeedsReveal = false
	}

	style := v.part(node, PopupPart, PartStyle{Fill: v.theme.Surface, Border: v.theme.Focus, Radius: 6})
	v.box(popupState, clip, clip, style.Fill, style.Border, style.Radius, 1)

	for index, option := range node.options {
		box := Rect{X: clip.X, Y: clip.Y + float32(index)*v.theme.ControlHeight - v.popupOffset, W: clip.W, H: v.theme.ControlHeight}
		if box.Intersect(clip).H <= 0 {
			continue
		}

		state := State(0)
		fill := Color{}

		if index == node.active {
			state |= Hovered
			fill = v.theme.Hover
		}

		if index == node.selected {
			state |= Selected
		}

		if option.Disabled {
			state |= Disabled
		}

		optionStyle := v.partState(node, OptionPart, state, PartStyle{Fill: fill, Radius: 4})
		if optionStyle.Fill[3] > 0 {
			v.box(popupState, box.Inset(3), clip, optionStyle.Fill, optionStyle.Border, optionStyle.Radius, 0)
		}

		color := v.theme.Foreground
		if option.Disabled {
			color = v.theme.Muted
		}

		layoutData, err := v.shape(option.Label, v.fontSize(node), 32768, false)
		if err != nil {
			return err
		}

		v.paintLayout(popupState, layoutData, box.X+12, box.Y+(box.H-layoutData.height)/2, clip, color)
	}

	return v.writeBlock(popupState, v.font.page)
}
