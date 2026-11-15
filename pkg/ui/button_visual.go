//nolint:mnd // Button visual size, gap, inset and opacity are explicit presentation metrics.
package ui

import (
	"strings"
	"unicode/utf8"
)

type ContentSide uint8

const (
	LeadingContent ContentSide = iota
	TrailingContent
)

// ButtonVisual is a decorative icon or prepared image beside a button's text.
// Empty button text centers the visual alone; SetLabel supplies its semantic name.
// Size defaults to 20 logical pixels and Gap to 8. Images preserve aspect ratio.
type ButtonVisual struct {
	Icon      string
	Image     *ImageAsset
	Side      ContentSide
	Size, Gap float32
}

// SetButtonVisual changes a button's decoration without adding an interaction
// target or focus stop. An empty value removes the decoration.
func (node *Node) SetButtonVisual(visual ButtonVisual) error {
	if (node.kind != Button && node.kind != ToggleButton) || validateButtonVisual(visual) != nil {
		return ErrBounds
	}

	if node.buttonVisual == visual {
		return nil
	}

	if node.view != nil {
		oldPage, newPage := buttonImage(node.buttonVisual), buttonImage(visual)
		if !node.view.canReplaceImage(oldPage, newPage) {
			return ErrBounds
		}

		node.view.releaseImage(oldPage)
		node.view.retainImage(newPage)
	}

	node.buttonVisual = visual
	node.damage(true)

	return nil
}

func (node *Node) ButtonVisual() ButtonVisual { return node.buttonVisual }

func validateButtonVisual(visual ButtonVisual) error {
	if visual.Side > TrailingContent || !utf8.ValidString(visual.Icon) || len(visual.Icon) > MaxTextBytes ||
		strings.ContainsAny(visual.Icon, "\r\n\t\x00") || (visual.Icon != "" && visual.Image != nil) {
		return ErrBounds
	}

	if !finite(visual.Size) || visual.Size < 0 || visual.Size > 256 || !finite(visual.Gap) || visual.Gap < 0 || visual.Gap > 32768 {
		return ErrBounds
	}

	if visual.Image != nil && !validImageAsset(visual.Image) {
		return ErrBounds
	}

	return nil
}

func (visual ButtonVisual) present() bool { return visual.Icon != "" || visual.Image != nil }
func (visual ButtonVisual) size() float32 {
	if visual.Size > 0 {
		return visual.Size
	}

	return 20
}
func (visual ButtonVisual) gap() float32 {
	if visual.Gap > 0 {
		return visual.Gap
	}

	return 8
}

func (v *View) measureButtonVisual(node *Node) float32 {
	visual := node.buttonVisual
	if !visual.present() {
		return 0
	}

	return visual.size() + 2*max(12, node.style.Padding)
}

func (v *View) paintButton(node *Node, box Rect, color Color) error {
	visual := node.buttonVisual
	if !visual.present() {
		return v.paintText(node, node.text, box, color)
	}

	color = v.part(node, LabelPart, PartStyle{Fill: color}).Fill

	label, err := v.shape(node.text, v.fontSize(node), 32768, false)
	if err != nil {
		return err
	}

	size := min(visual.size(), max(0, box.W), max(0, box.H))

	gap := float32(0)
	if node.text != "" {
		gap = min(visual.gap(), max(0, box.W-size))
	}

	labelWidth := min(textExtent(label), max(0, box.W-size-gap))
	start := box.X + (box.W-size-gap-labelWidth)/2

	labelX, visualX := start+size+gap, start
	if visual.Side == TrailingContent {
		labelX, visualX = start, start+labelWidth+gap
	}

	clip := node.clip.Intersect(node.rect)

	centerY := node.rect.Y + node.rect.H/2
	if node.text != "" && labelWidth > 0 {
		labelClip := clip.Intersect(Rect{X: labelX, Y: node.rect.Y, W: labelWidth, H: node.rect.H})
		labelY := box.Y + (box.H-label.height)/2
		v.paintLayout(node, label, labelX, labelY, labelClip, color)

		// The line box includes leading and descent; its geometric center is
		// different from the visible label. Keep the text baseline and align
		// the decoration with its glyph bounds instead.
		bounds := glyphBounds(label)
		if bounds.W > 0 && bounds.H > 0 {
			centerY = labelY + bounds.Y + bounds.H/2
		}
	}

	centerY = max(node.rect.Y+size/2, min(node.rect.Y+node.rect.H-size/2, centerY))

	visualBox := Rect{X: visualX, Y: centerY - size/2, W: size, H: size}
	if visual.Image != nil {
		v.paintButtonImage(node, visualBox, clip)

		return nil
	}

	return v.paintButtonIcon(node, visualBox, clip, color)
}

func (v *View) paintButtonIcon(node *Node, box, clip Rect, color Color) error {
	icon, err := v.shape(node.buttonVisual.Icon, node.buttonVisual.size(), 32768, false)
	if err != nil {
		return err
	}

	bounds := glyphBounds(icon)
	if bounds.W <= 0 || bounds.H <= 0 || box.W <= 0 || box.H <= 0 {
		return nil
	}

	color = v.part(node, IconPart, PartStyle{Fill: color}).Fill
	scale := min(box.W/bounds.W, box.H/bounds.H)
	positionX := box.X + (box.W-bounds.W*scale)/2 - bounds.X*scale
	positionY := box.Y + (box.H-bounds.H*scale)/2 - bounds.Y*scale

	for _, glyph := range icon.glyphs {
		record := glyph.rect
		record.X, record.Y = positionX+record.X*scale, positionY+record.Y*scale
		record.W, record.H = record.W*scale, record.H*scale
		node.paint = append(node.paint, paintRecord{
			rect: record, clip: clip.Intersect(box), color: color, uv: glyph.uv, mode: 2, pxRange: v.font.pxRange,
		})
	}

	return nil
}

func (v *View) paintButtonImage(node *Node, box, clip Rect) {
	asset := node.buttonVisual.Image
	scale := min(box.W/float32(asset.width), box.H/float32(asset.height))
	width, height := float32(asset.width)*scale, float32(asset.height)*scale
	rect := Rect{X: box.X + (box.W-width)/2, Y: box.Y + (box.H-height)/2, W: width, H: height}

	color := v.part(node, ImagePart, PartStyle{Fill: Color{1, 1, 1, 1}}).Fill
	if inheritedDisabled(node) {
		color[3] *= 0.4
	}

	node.paint = append(
		node.paint,
		paintRecord{rect: rect, clip: clip.Intersect(box), color: color, uv: asset.uv, mode: 1, texture: asset.page},
	)
}

func textExtent(layoutData textLayout) float32 {
	width := float32(0)
	for _, position := range layoutData.carets {
		width = max(width, position.positionX)
	}

	return width
}

func glyphBounds(layoutData textLayout) Rect {
	if len(layoutData.glyphs) == 0 {
		return Rect{}
	}

	bounds := layoutData.glyphs[0].rect
	for _, glyph := range layoutData.glyphs[1:] {
		right, bottom := max(bounds.X+bounds.W, glyph.rect.X+glyph.rect.W), max(bounds.Y+bounds.H, glyph.rect.Y+glyph.rect.H)
		bounds.X, bounds.Y = min(bounds.X, glyph.rect.X), min(bounds.Y, glyph.rect.Y)
		bounds.W, bounds.H = right-bounds.X, bottom-bounds.Y
	}

	return bounds
}
