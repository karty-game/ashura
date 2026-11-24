// Package uihost defines the value-only boundary between retained UI and its hosts.
// It contains no controls, layout, fonts or text shaping.
package uihost

import (
	"time"

	"github.com/karty-game/ashura/internal/uipaint"
)

// MaxTextBytes bounds a retained text value and platform character-bound requests.
const MaxTextBytes = 4096

// View supplies the UI operations consumed by application and platform hosts.
//
//nolint:interfacebloat // One optional attachment supplies the complete retained surface to its application and platform hosts.
type View interface {
	Resize(width, height float32) error
	Frame(now time.Duration) (*uipaint.Frame, error)
	CapturesKeyboard() bool
	CapturesPointer() bool
	ConsumesPointer(positionX, positionY float32) bool
	PreventsPointerLock() bool
	CapturedPointer() (int64, bool)
	Dispatch(event Event) Result
	DispatchPreview(event Event) Result
	FocusedID() ID
	Editing() EditingState
	CharacterBounds(offset int) Rect
	SetCapabilities(capabilities Capabilities)
	Capabilities() Capabilities
	SetReducedMotion(reduced bool)
	CursorAt(positionX, positionY float32) Cursor
	Stats() Stats
}

// Bridge releases the platform-specific resources of one attached UI.
type Bridge interface {
	Close()
}

// Attachment opts a UI into an application and creates its platform bridge.
// The host is the backend-owned driver for the selected build target.
type Attachment interface {
	View
	AttachUI(host any) (Bridge, error)
}

type ID uint64
type EventKind uint8

const (
	PointerDown EventKind = iota + 1
	PointerMove
	PointerUp
	PointerCancel
	Wheel
	KeyDown
	KeyUp
	InsertText
	ReplaceText
	SetSelection
	CompositionStart
	CompositionEnd
	CompositionCancel
	Blur
	CaptureLost
	Activate
	Increment
	Decrement
)

type Modifiers struct{ Shift, Control, Alt, Meta bool }

// Event text offsets are UTF-8 byte positions, never rune/glyph/UTF-16 indices.
// Session/Revision zero accepts the current synchronous editing transaction.
type Event struct {
	Kind                       EventKind
	Pointer                    int64
	Device                     string
	X, Y, DeltaX, DeltaY       float32
	Button                     int
	Key, Code, Text            string
	Modifiers                  Modifiers
	Repeat                     bool
	Start, End, Anchor, Cursor int
	Session                    ID
	Revision                   uint64
	Target                     ID
	Time                       time.Duration
}

// Result contains immediate platform requests produced by event dispatch.
type Result struct {
	Err                           error
	Consumed, Capture, Release    bool
	Pointer                       int64
	Copy                          string
	ClipboardWrite, ClipboardRead bool
	Session                       ID
	Revision                      uint64
}

type Capabilities struct{ TextInput, Composition, Clipboard, Capture, Accessibility bool }

type EditingState struct {
	Active         bool
	Session        ID
	Revision       uint64
	Text           string
	Anchor, Cursor int
	Bounds, Caret  Rect
	Composing      bool
}

// Cursor is the platform cursor requested by a UI surface.
type Cursor string

const (
	DefaultCursor          Cursor = "default"
	PointerCursor          Cursor = "pointer"
	TextCursor             Cursor = "text"
	HorizontalResizeCursor Cursor = "ew-resize"
	VerticalResizeCursor   Cursor = "ns-resize"
)

type Stats struct {
	Layouts, Shapes, Paints, Measures uint64
	Records, Nodes                    int
}

type Rect struct{ X, Y, W, H float32 }

func (record Rect) Contains(positionX, positionY float32) bool {
	return positionX >= record.X && positionY >= record.Y && positionX < record.X+record.W && positionY < record.Y+record.H
}

func (record Rect) Intersect(bound Rect) Rect {
	positionX, positionY := max(record.X, bound.X), max(record.Y, bound.Y)

	return Rect{
		positionX,
		positionY,
		max(0, min(record.X+record.W, bound.X+bound.W)-positionX),
		max(0, min(record.Y+record.H, bound.Y+bound.H)-positionY),
	}
}

// Inset returns the bounds after removing the same amount from each edge.
func (record Rect) Inset(amount float32) Rect {
	return Rect{record.X + amount, record.Y + amount, max(0, record.W-2*amount), max(0, record.H-2*amount)}
}

func (record Rect) Array() [4]float32 { return [4]float32{record.X, record.Y, record.W, record.H} }
