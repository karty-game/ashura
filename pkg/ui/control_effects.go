//nolint:mnd // Ripple opacity and focus-indicator inset/thickness are presentation metrics.
package ui

func (v *View) centerControlRipple(node *Node) {
	v.startControlRipple(node, node.rect.X+node.rect.W/2, node.rect.Y+node.rect.H/2)
}

func (v *View) pulseControlRipple(node *Node) {
	v.centerControlRipple(node)
	v.releaseControlRipple(node)
}

func (v *View) paintControlRipple(node *Node, radius, borderWidth float32, color Color) {
	positionX, positionY, extent, opacity, visible := v.controlRipple(node)
	if !visible {
		return
	}

	color[3] *= 0.06
	style := v.part(node, RipplePart, PartStyle{Fill: color})
	style.Fill[3] *= opacity
	bounds := node.rect.Inset(borderWidth)
	node.paint = append(node.paint, paintRecord{
		rect: bounds, clip: node.clip.Intersect(node.rect), color: style.Fill,
		uv: Rect{X: positionX, Y: positionY}, radius: max(0, radius-borderWidth), borderWidth: extent, mode: 6,
	})
}

func (v *View) paintInputIndicator(node *Node) {
	focus := v.controlFocus(node)
	if focus <= 0 {
		return
	}

	color := v.theme.Accent
	if node.invalid {
		color = v.theme.Danger
	}

	radius := node.style.Radius
	if radius == 0 {
		radius = v.theme.Radius
	}

	width := max(0, node.rect.W-2*max(4, radius)) * focus
	bounds := Rect{X: node.rect.X + (node.rect.W-width)/2, Y: node.rect.Y + node.rect.H - 2, W: width, H: 2}
	style := v.part(node, ActiveIndicatorPart, PartStyle{Fill: color, Radius: 1})
	v.box(node, bounds, node.clip.Intersect(node.rect), style.Fill, style.Border, style.Radius, 0)
}
