//nolint:mnd // Feedback dimensions and tint weights are explicit theme defaults.
package ui

import (
	"math"
	"unicode/utf8"
)

// AlertTone identifies severity independently of a theme's colors.
type AlertTone uint8

const (
	InfoAlert AlertTone = iota + 1
	SuccessAlert
	WarningAlert
	DangerAlert
)

// NewSeparator creates a decorative horizontal rule, excluded from focus traversal.
func NewSeparator() *Node {
	return &Node{kind: Separator}
}

// NewAlert creates persistent, nonmodal feedback. Its severity is also written in text.
// Info and success use status semantics; warning and danger use alert semantics.
func NewAlert(tone AlertTone, title, message string) *Node {
	prefix, role := alertDescription(tone)
	if prefix == "" || len(title) > MaxTextBytes-len(prefix)-len(" · ") || len(message) > MaxTextBytes || !utf8.ValidString(title) ||
		!utf8.ValidString(message) {
		return &Node{kind: Panel, constructionErr: ErrBounds}
	}

	heading := prefix
	if title != "" {
		heading += " · " + title
	}

	children := make([]*Node, 0, 2)

	children = append(children, NewText(heading))
	if message != "" {
		children = append(children, NewText(message))
	}

	node := Column(children...).WithStyle(Style{Padding: 16, Gap: 6, Radius: 8, BorderWidth: 1})
	node.role, node.label, node.description, node.alertTone = role, title, message, tone

	return node
}

// NewProgress creates a determinate 0..1 indicator. Its label names the semantic control.
// Compose Text beside or above it when a visible label or percentage is desired.
func NewProgress(label string, value float64) *Node {
	node := &Node{kind: Progress, label: label, maximum: 1, readOnly: true}
	node.constructionErr = node.SetProgress(value)

	return node
}

// Progress returns the indicator's completion fraction.
func (node *Node) Progress() float64 { return node.value }

// SetProgress silently updates a determinate indicator and rejects values outside 0..1.
func (node *Node) SetProgress(value float64) error {
	if node.kind != Progress || math.IsNaN(value) || math.IsInf(value, 0) || value < 0 || value > 1 {
		return ErrBounds
	}

	if node.value != value {
		node.value = value
		node.damage(false)
	}

	return nil
}

func (v *View) paintProgress(node *Node, box Rect) {
	height := min(float32(8), node.rect.H)
	track := Rect{X: box.X, Y: node.rect.Y + (node.rect.H-height)/2, W: box.W, H: height}
	background := v.part(node, Track, PartStyle{Fill: v.theme.Control, Border: v.theme.Border, Radius: height / 2})
	v.box(node, track, node.clip, background.Fill, background.Border, background.Radius, 1)

	if node.value == 0 {
		return
	}

	indicator := v.part(node, Indicator, PartStyle{Fill: v.theme.Accent, Radius: height / 2})
	if inheritedDisabled(node) {
		indicator.Fill = v.theme.Muted
	}

	track.W *= float32(node.value)
	v.box(node, track, node.clip, indicator.Fill, indicator.Border, indicator.Radius, 0)
}

func (v *View) paintSeparator(node *Node) {
	color := node.style.Fill
	if color == (Color{}) {
		color = v.theme.Border
	}

	box := node.rect.Inset(node.style.Padding)
	box.Y += max(0, (box.H-1)/2)
	box.H = min(1, box.H)
	v.box(node, box, node.clip, color, Color{}, 0, 0)
}

func (v *View) alertPaint(node *Node, fill, foreground, border Color) (Color, Color, Color) {
	if node.alertTone != 0 {
		tone := v.alertColor(node.alertTone)
		if fill == (Color{}) {
			fill = alertTint(v.theme.Surface, tone)
		}

		if border == (Color{}) {
			border = tone
		}
	} else if node.parent != nil && node.parent.alertTone != 0 && len(node.parent.children) > 0 && node == node.parent.children[0] &&
		node.style.Foreground == (Color{}) {
		foreground = v.alertColor(node.parent.alertTone)
	}

	return fill, foreground, border
}

func (v *View) alertColor(tone AlertTone) Color {
	var color Color

	switch tone {
	case InfoAlert:
		color = v.theme.Info
	case SuccessAlert:
		color = v.theme.Success
	case WarningAlert:
		color = v.theme.Warning
	case DangerAlert:
		color = v.theme.Danger
	}

	if color == (Color{}) {
		color = v.theme.Accent
	}

	return color
}

func alertDescription(tone AlertTone) (string, string) {
	switch tone {
	case InfoAlert:
		return "Info", "status"
	case SuccessAlert:
		return "Success", "status"
	case WarningAlert:
		return "Warning", "alert"
	case DangerAlert:
		return "Error", "alert"
	}

	return "", ""
}

func alertTint(background, tone Color) Color {
	for index := range background {
		background[index] = background[index]*0.88 + tone[index]*0.12
	}

	return background
}
