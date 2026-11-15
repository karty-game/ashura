//nolint:mnd // UI metrics, coordinate equations and bounded prepared-format widths are explicit.
package ui

import (
	"unicode/utf8"

	"github.com/karty-game/karty-webgpu/internal/uipaint"
)

type paintRecord struct {
	texture                            *uipaint.Texture
	rect, clip                         Rect
	color, border                      Color
	uv                                 Rect
	radius, borderWidth, mode, pxRange float32
}

func (v *View) box(node *Node, rect, clip Rect, fill, border Color, radius, width float32) {
	node.paint = append(node.paint, paintRecord{rect: rect, clip: clip, color: fill, border: border, radius: radius, borderWidth: width})
}
func (v *View) writeBlock(node *Node, texture *uipaint.Texture) error {
	count := len(node.paint)
	if node.capacity == 0 || count > node.capacity {
		block, err := v.allocateRecords(count)
		if err != nil {
			return err
		}

		if node.capacity > 0 {
			v.free = append(v.free, recordBlock{node.slot, node.capacity})
		}

		node.slot, node.capacity = block.slot, block.capacity

		if len(v.frame.Data) < v.used*uipaint.Stride {
			v.frame.Data = append(v.frame.Data, make([]byte, v.used*uipaint.Stride-len(v.frame.Data))...)
		}
	}

	for index, record := range node.paint {
		uipaint.Record(
			v.frame.Data[(node.slot+index)*uipaint.Stride:],
			record.rect.Array(),
			record.clip.Array(),
			record.color,
			record.border,
			record.uv.Array(),
			[4]float32{record.radius, record.borderWidth, record.mode, record.pxRange},
		)
	}

	v.updateBatches(node, texture)

	if count > 0 {
		v.frame.Changes = append(v.frame.Changes, uipaint.Range{Offset: node.slot * uipaint.Stride, Size: count * uipaint.Stride})
	}

	return nil
}
func (v *View) paintNode(node *Node) error {
	v.stats.Paints++

	node.paint = node.paint[:0]
	if node.rect.Intersect(node.clip).W <= 0 || node.rect.Intersect(node.clip).H <= 0 {
		return v.writeBlock(node, v.font.page)
	}

	fill, foreground, border, radius, width := v.nodePaint(node)
	v.paintNodeSurface(node, fill, border, radius, width)

	v.paintControlRipple(node, radius, width, foreground)

	inner := node.rect.Inset(max(12, node.style.Padding))

	switch node.kind {
	case Chart:
		if err := v.paintChart(node, inner); err != nil {
			return err
		}
	case Button, ToggleButton:
		if err := v.paintButton(node, inner, foreground); err != nil {
			return err
		}
	case Progress, Separator:
		v.paintFeedback(node, inner)
	case Panel:
		v.paintScrollbar(node)
	case Image:
		node.paint = append(node.paint, paintRecord{rect: node.rect, clip: node.clip, color: Color{1, 1, 1, 1}, uv: node.imageUV, mode: 1})

		return v.writeBlock(node, node.image)
	case Text:
		v.paintLayout(node, node.layout, node.rect.X+node.style.Padding, node.rect.Y+node.style.Padding, node.clip, foreground)
	case Checkbox, Switch:
		if err := v.paintBoolean(node, inner, foreground); err != nil {
			return err
		}
	case Slider:
		v.paintSlider(node, inner)
	case Input:
		if err := v.paintInput(node, inner, foreground); err != nil {
			return err
		}
	case Select:
		if err := v.paintSelect(node, inner, foreground); err != nil {
			return err
		}
	}

	return v.writeBlock(node, v.font.page)
}

func (v *View) paintNodeSurface(node *Node, fill, border Color, radius, width float32) {
	if node.kind != Panel || node.panelBackground.Image == nil {
		if fill[3] > 0 || width > 0 {
			v.box(node, node.rect, node.clip, fill, border, radius, width)
		}

		return
	}

	if fill[3] > 0 {
		v.box(node, node.rect, node.clip, fill, Color{}, radius, 0)
	}

	v.paintPanelBackground(node)

	if width > 0 {
		v.box(node, node.rect, node.clip, Color{}, border, radius, width)
	}
}

func (v *View) paintSelect(node *Node, box Rect, color Color) error {
	text := "Choose an option"
	if node.selected >= 0 {
		text = node.options[node.selected].Label
	}

	if err := v.paintText(node, text, box, color); err != nil {
		return err
	}

	positionX, positionY := node.rect.X+node.rect.W-20, node.rect.Y+node.rect.H/2
	for index := range 3 {
		v.box(
			node,
			Rect{X: positionX + float32(index), Y: positionY + float32(index), W: 7 - float32(index*2), H: 1},
			node.clip,
			color,
			Color{},
			0,
			0,
		)
	}

	return nil
}
func (v *View) paintText(node *Node, text string, box Rect, color Color) error {
	color = v.part(node, LabelPart, PartStyle{Fill: color}).Fill

	layoutData, err := v.shape(text, v.fontSize(node), max(1, box.W), false)
	if err != nil {
		return err
	}

	v.paintLayout(node, layoutData, box.X, box.Y+(box.H-layoutData.height)/2, node.clip.Intersect(node.rect), color)

	return nil
}
func (v *View) paintLayout(node *Node, layoutData textLayout, positionX, positionY float32, clip Rect, color Color) {
	for _, glyph := range layoutData.glyphs {
		record := glyph.rect
		record.X += positionX

		record.Y += positionY
		if record.W <= 0 || record.H <= 0 || record.X+record.W <= clip.X || record.X >= clip.X+clip.W ||
			record.Y+record.H <= clip.Y || record.Y >= clip.Y+clip.H {
			continue
		}

		node.paint = append(node.paint, paintRecord{rect: record, clip: clip, color: color, uv: glyph.uv, mode: 2, pxRange: v.font.pxRange})
	}
}
func (v *View) paintBoolean(node *Node, box Rect, color Color) error {
	width, height := float32(22), float32(22)
	if node.kind == Switch {
		width = 40
	}

	mark := Rect{X: box.X, Y: node.rect.Y + (node.rect.H-height)/2, W: width, H: height}

	fill := v.theme.Surface
	if node.checked || node.mixed {
		fill = v.theme.Accent
	}

	part := v.part(node, Track, PartStyle{Fill: fill, Border: v.theme.Border, Radius: height / 2})
	if node.kind == Checkbox {
		part = v.part(node, Indicator, PartStyle{Fill: fill, Border: v.theme.Border, Radius: 5})
	}

	v.box(node, mark, node.clip, part.Fill, part.Border, part.Radius, 1)

	if node.kind == Switch {
		positionX := mark.X + 3
		if node.checked {
			positionX = mark.X + mark.W - 19
		}

		thumb := v.part(node, Thumb, PartStyle{Fill: v.theme.Foreground, Radius: 8})
		v.box(node, Rect{X: positionX, Y: mark.Y + 3, W: 16, H: 16}, node.clip, thumb.Fill, thumb.Border, thumb.Radius, 0)
	} else if node.checked || node.mixed {
		record := mark.Inset(6)
		if node.mixed {
			record.Y += 4
			record.H = 2
		}

		v.box(node, record, node.clip, Hex(0x10252c), Color{}, 1, 0)
	}

	box.X += width + 10
	box.W = max(0, box.W-width-10)

	return v.paintText(node, node.label, box, color)
}
func (v *View) paintSlider(node *Node, box Rect) {
	fraction := float32(0)
	if node.maximum > node.minimum {
		fraction = float32((node.value - node.minimum) / (node.maximum - node.minimum))
	}

	positionY := node.rect.Y + node.rect.H/2
	track := v.part(node, Track, PartStyle{Fill: v.theme.Border, Radius: 3})
	v.box(node, Rect{X: box.X, Y: positionY - 3, W: box.W, H: 6}, node.clip, track.Fill, track.Border, track.Radius, 0)
	v.box(node, Rect{X: box.X, Y: positionY - 3, W: box.W * fraction, H: 6}, node.clip, v.theme.Accent, Color{}, 3, 0)
	thumb := v.part(node, Thumb, PartStyle{Fill: v.theme.Accent, Radius: 9})
	v.box(node, Rect{X: box.X + fraction*box.W - 9, Y: positionY - 9, W: 18, H: 18}, node.clip, thumb.Fill, thumb.Border, thumb.Radius, 0)
}

//nolint:funlen // One input paint composes selection, glyphs, placeholder, caret and composition underline in painter order.
func (v *View) paintInput(node *Node, box Rect, color Color) error {
	v.paintInputIndicator(node)

	layoutData, err := v.shape(node.text, v.fontSize(node), 32768, false)
	if err != nil {
		return err
	}

	node.layout = layoutData
	clip := node.clip.Intersect(Rect{X: box.X, Y: node.rect.Y, W: box.W, H: node.rect.H})

	positionX, positionY := layoutData.position(node.cursor)
	if v.focus == node {
		if positionX-node.editOffset > box.W-2 {
			node.editOffset = positionX - box.W + 2
		}

		if positionX < node.editOffset {
			node.editOffset = positionX
		}

		node.editOffset = max(0, node.editOffset)
	}

	baseY := node.rect.Y + (node.rect.H-layoutData.height)/2
	if v.focus == node && node.anchor != node.cursor {
		left, right := min(node.anchor, node.cursor), max(node.anchor, node.cursor)
		for index := left; index < right; {
			_, size := utf8.DecodeRuneInString(node.text[index:])
			x1, y1 := layoutData.position(index)
			x2, _ := layoutData.position(index + size)
			v.box(
				node,
				Rect{X: box.X + min(x1, x2) - node.editOffset, Y: baseY + y1, W: max(1, abs(x2-x1)), H: v.fontSize(node) * 1.4},
				clip,
				v.theme.Selection,
				Color{},
				0,
				0,
			)

			index += size
		}
	}

	if node.text == "" && node.placeholder != "" {
		placeholder, event := v.shape(node.placeholder, v.fontSize(node), 32768, false)
		if event != nil {
			return event
		}

		v.paintLayout(node, placeholder, box.X, baseY, clip, v.theme.Muted)
	} else {
		v.paintLayout(node, layoutData, box.X-node.editOffset, baseY, clip, color)
	}

	if v.focus == node {
		caretColor := color
		if !v.caretVisible {
			caretColor[3] = 0
		}

		v.box(
			node,
			Rect{X: box.X + positionX - node.editOffset, Y: baseY + positionY, W: 1.5, H: v.fontSize(node) * 1.4},
			clip,
			caretColor,
			Color{},
			0,
			0,
		)
	}

	if node.composing {
		start, _ := layoutData.position(node.compositionStart)
		v.box(
			node,
			Rect{X: box.X + start - node.editOffset, Y: baseY + v.fontSize(node)*1.35, W: max(2, positionX-start), H: 1},
			clip,
			v.theme.Focus,
			Color{},
			0,
			0,
		)
	}

	return nil
}
func abs(v float32) float32 {
	if v < 0 {
		return -v
	}

	return v
}

func (v *View) nodePaint(node *Node) (Color, Color, Color, float32, float32) {
	theme := v.theme

	fill, foreground, border := node.style.Fill, theme.Foreground, node.style.Border
	if node.style.Foreground != (Color{}) {
		foreground = node.style.Foreground
	}

	if node.style.Foreground == (Color{}) {
		if node.fieldHint {
			foreground = theme.Muted
		}

		if node.fieldError {
			foreground = theme.Danger
		}
	}

	radius := node.style.Radius
	if !isControl(node.kind) {
		fill, foreground, border = v.alertPaint(node, fill, foreground, border)

		width := node.style.BorderWidth
		if node.focusable && v.focus == node && v.focusVisible {
			border = theme.Focus
			width = 2
		}

		return fill, foreground, border, radius, width
	}

	fill, foreground, border = v.controlPaint(node, fill, foreground, border)

	if radius == 0 {
		radius = theme.Radius
	}

	width := node.style.BorderWidth
	if node.kind == Input {
		if border == (Color{}) {
			border = theme.Border
		}

		width = max(1, width)

		if v.focus == node {
			border = theme.Accent
			width = max(2, width)
		}
	}

	if (node.kind == Button || node.kind == ToggleButton) && node.buttonVariant != QuietButton && width == 0 {
		width = 1
	}

	if node.invalid {
		border = theme.Danger
		width = max(1, width)
	}

	fill, foreground, border, radius, width = v.animateControl(node, fill, foreground, border, radius, width)

	if v.focus == node && v.focusVisible {
		border = theme.Focus
		width = 2
	}

	return fill, foreground, border, radius, width
}

func (v *View) paintScrollbar(node *Node) {
	if !node.style.Scroll || scrollExtent(node) <= 0 {
		return
	}

	_, thumb := scrollbarGeometry(node)

	color := v.theme.Border
	if v.capture != nil && v.capture.node == node && v.capture.scrollbar {
		color = v.theme.Accent
	}

	v.box(node, thumb, node.clip, color, Color{}, 2, 0)
}

func (v *View) paintFeedback(node *Node, box Rect) {
	if node.kind == Progress {
		v.paintProgress(node, box)
	} else {
		v.paintSeparator(node)
	}
}
