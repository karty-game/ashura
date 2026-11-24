// Package ui provides retained, styled controls without platform or renderer dependencies.
//
//nolint:mnd // UI metrics, coordinate equations and bounded prepared-format widths are explicit.
package ui

import (
	"errors"
	"math"

	"github.com/karty-game/ashura/internal/uihost"
	"github.com/karty-game/ashura/internal/uipaint"
)

const (
	MaxNodes      = 4096
	MaxDepth      = 32
	MaxTextBytes  = uihost.MaxTextBytes
	MaxRecords    = 65536
	MaxImageBytes = 128 << 20
)

var ErrBounds = errors.New("ui: invalid value or resource limit exceeded")
var ErrReentrant = errors.New("ui: nested input dispatch is unsupported; update nodes in callbacks instead")

type ID = uihost.ID
type Kind uint8

const (
	Panel Kind = iota
	Text
	Image
	Button
	Checkbox
	Switch
	ToggleButton
	Slider
	Select
	Input
	Progress
	Separator
	Chart
)

type Rect = uihost.Rect

func finite(v float32) bool { return !math.IsNaN(float64(v)) && !math.IsInf(float64(v), 0) }

// Color is unpremultiplied sRGB, independent of an engine color convention.
type Color [4]float32

func Hex(rgb uint32) Color {
	return Color{float32(rgb>>16&255) / 255, float32(rgb>>8&255) / 255, float32(rgb&255) / 255, 1}
}

type Direction uint8

const (
	Vertical Direction = iota + 1
	Horizontal
)

// Style is a typed layout/paint value. Zero width stretches; zero height is intrinsic.
type Style struct {
	Basis                                       float32
	Wrap                                        bool
	Width, Height, MinWidth, MaxWidth, Grow     float32
	Padding, Gap, Radius, BorderWidth, FontSize float32
	Fill, Foreground, Border                    Color
	Direction                                   Direction
	Scroll                                      bool
}

type Theme struct {
	Motion                                                                                            MotionTheme
	PrimaryButton, SecondaryButton                                                                    ButtonPalette
	Info, Success, Warning, Danger                                                                    Color
	Background, Surface, Control, Hover, Pressed, Foreground, Muted, Accent, Focus, Border, Selection Color
	Radius, FontSize, ControlHeight, Gap                                                              float32
}

func DarkTheme() Theme {
	return Theme{
		Motion: defaultMotionTheme(),
		PrimaryButton: ButtonPalette{
			Fill:       Hex(0x6bdac5),
			Hover:      Hex(0x75decb),
			Pressed:    Hex(0x61cdb9),
			Foreground: Hex(0x10252c),
			Border:     Hex(0x6bdac5),
		},
		SecondaryButton: ButtonPalette{
			Fill:       Hex(0x304d70),
			Hover:      Hex(0x355578),
			Pressed:    Hex(0x2b4565),
			Foreground: Hex(0xe9f0fa),
			Border:     Hex(0x668bb6),
		},
		Info:          Hex(0x85bbff),
		Success:       Hex(0x6bdac5),
		Warning:       Hex(0xf4cc76),
		Danger:        Hex(0xff9caa),
		Background:    Hex(0x0c121e),
		Surface:       Hex(0x141f30),
		Control:       Hex(0x202e43),
		Hover:         Hex(0x26364c),
		Pressed:       Hex(0x1c293c),
		Foreground:    Hex(0xe9f0fa),
		Muted:         Hex(0x9caec4),
		Accent:        Hex(0x6bdac5),
		Focus:         Hex(0xf4cc76),
		Border:        Hex(0x40546f),
		Selection:     Color{0.24, 0.49, 0.70, 0.7},
		Radius:        8,
		FontSize:      16,
		ControlHeight: 42,
		Gap:           12,
	}
}

type Change struct {
	Node               *Node
	Text               string
	Value              float64
	Checked, Composing bool
}
type Option struct {
	Key, Label string
	Disabled   bool
}

type snapshot struct {
	text           string
	anchor, cursor int
}

// Node identity survives value/style changes. Mutate through setters for damage tracking.
type Node struct {
	virtual                                    *virtualList
	virtualInactive                            bool
	virtualIndex, virtualSize                  int
	panelBackground                            PanelBackground
	motion                                     controlMotion
	chart                                      *chartState
	layoutRevision                             uint64
	field                                      *fieldState
	invalid, fieldHint, fieldError             bool
	describedBy                                *Node
	current                                    string
	flow                                       []flowItem
	flowCaps                                   []flowCap
	flowLines                                  []flowLine
	tabs                                       *tabGroup
	pagination                                 *paginationState
	role                                       string
	tabSkip, focusable, semanticSelected       bool
	controls, labelledBy                       *Node
	buttonVariant                              ButtonVariant
	buttonVisual                               ButtonVisual
	alertTone                                  AlertTone
	tooltip                                    string
	overlay                                    bool
	constructionErr                            error
	id                                         ID
	kind                                       Kind
	view                                       *View
	parent                                     *Node
	children                                   []*Node
	style                                      Style
	text, label, description                   string
	options                                    []Option
	selected, active                           int
	value, minimum, maximum, step              float64
	checked, mixed, disabled, hidden, readOnly bool
	activate                                   func()
	change, commit                             func(Change)
	cancel                                     func()
	preview, bubble                            func(*RoutedEvent)
	rect, clip                                 Rect
	naturalW, naturalH, contentHeight, offset  float32
	layout                                     textLayout
	placeholder                                string
	anchor, cursor                             int
	editOffset                                 float32
	undo, redo                                 []snapshot
	composing                                  bool
	composition                                snapshot
	compositionStart, compositionEnd           int
	revision                                   uint64
	image                                      *uipaint.Texture
	imageUV                                    Rect
	dirty                                      bool
	batch, slot, capacity                      int
	paint                                      []paintRecord
	batches                                    []uipaint.Batch
	parts                                      []partRule
}

func (node *Node) ID() ID         { return node.id }
func (node *Node) Kind() Kind     { return node.kind }
func (node *Node) Bounds() Rect   { return node.rect }
func (node *Node) Text() string   { return node.text }
func (node *Node) Value() float64 { return node.value }
func (node *Node) Checked() bool  { return node.checked }
func (node *Node) Selected() string {
	if node.selected >= 0 && node.selected < len(node.options) {
		return node.options[node.selected].Key
	}

	return ""
}
func (node *Node) Style() Style      { return node.style }
func (node *Node) Children() []*Node { return append([]*Node(nil), node.children...) }

type EventKind = uihost.EventKind

const (
	PointerDown       = uihost.PointerDown
	PointerMove       = uihost.PointerMove
	PointerUp         = uihost.PointerUp
	PointerCancel     = uihost.PointerCancel
	Wheel             = uihost.Wheel
	KeyDown           = uihost.KeyDown
	KeyUp             = uihost.KeyUp
	InsertText        = uihost.InsertText
	ReplaceText       = uihost.ReplaceText
	SetSelection      = uihost.SetSelection
	CompositionStart  = uihost.CompositionStart
	CompositionEnd    = uihost.CompositionEnd
	CompositionCancel = uihost.CompositionCancel
	Blur              = uihost.Blur
	CaptureLost       = uihost.CaptureLost
	Activate          = uihost.Activate
	Increment         = uihost.Increment
	Decrement         = uihost.Decrement
)

type Modifiers = uihost.Modifiers

// Event text offsets are UTF-8 byte positions, never rune/glyph/UTF-16 indices.
// Session/Revision zero accepts the current synchronous editing transaction.
type Event = uihost.Event

type RoutedEvent struct {
	Event              Event
	Target, Current    *Node
	stopped, prevented bool
}

func (event *RoutedEvent) StopPropagation() { event.stopped = true }
func (event *RoutedEvent) PreventDefault()  { event.prevented = true }

// Result returns immediate wrapper requests; no browser calls occur in the core.
type Result = uihost.Result
type Capabilities = uihost.Capabilities
type EditingState = uihost.EditingState

func isControl(kind Kind) bool { return kind >= Button && kind <= Input }
