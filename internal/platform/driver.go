// Package platform defines the private boundary shared by native and browser drivers.
package platform

import (
	"context"
	"image"
	"time"

	"github.com/karty-game/karty-webgpu/internal/bufferdata"
	"github.com/karty-game/karty-webgpu/internal/computedata"
	"github.com/karty-game/karty-webgpu/internal/diagnostics"
	"github.com/karty-game/karty-webgpu/internal/render"
	"github.com/karty-game/karty-webgpu/internal/texturedata"
	"github.com/karty-game/karty-webgpu/internal/uihost"
)

// Config contains platform-independent application window settings.
type Config struct {
	UI       uihost.Attachment
	Features []Feature
	// PointShadowResolution declares the fixed point-shadow resources before Run.
	PointShadowResolution uint32
	Metrics               *diagnostics.Monitor
	SampleCount           uint32
	Title                 string
	Width                 int
	Height                int
	CanvasID              string
}

const (
	KeyCode1     uint8 = '1'
	KeyCode2     uint8 = '2'
	KeyCode3     uint8 = '3'
	KeyCodeA     uint8 = 'A'
	KeyCodeB     uint8 = 'B'
	KeyCodeH     uint8 = 'H'
	KeyCodeL     uint8 = 'L'
	KeyCodeR     uint8 = 'R'
	KeyCodeD     uint8 = 'D'
	KeyCodeN     uint8 = 'N'
	KeyCodeO     uint8 = 'O'
	KeyCodeS     uint8 = 'S'
	KeyCodeT     uint8 = 'T'
	KeyCodeW     uint8 = 'W'
	KeyCodeSpace uint8 = ' '
	KeyCodeShift uint8 = 0x80
)

// InputState is the normalized input snapshot for one frame.
type InputState struct {
	LogicalWidth float32
	// LogicalHeight is the viewport height in CSS/window pixels, before DPR scaling.
	LogicalHeight float32
	// Touch deltas accumulate between frames, in logical pixels. Pinch is a log ratio.
	TouchDX, TouchDY     float32
	TouchPanX, TouchPanY float32
	Pinch                float32
	Width                int
	Height               int
	Keys                 [256]bool
	// Drag deltas retain completed drags until the frame consumes them.
	DragDX, DragDY [8]float32
	MouseButtons   [8]bool
	MouseX         float32
	MouseY         float32
	MouseDX        float32
	MouseDY        float32
	PointerLocked  bool
	WheelX         float32
	WheelY         float32
}

// Driver owns platform event handling, the native/browser frame schedule and
// backend-specific WebGPU resources.
type Driver interface {
	PrepareCompute(resources computedata.Resources) error
	UnloadBuffer(buffer *bufferdata.Buffer)
	UnloadComputePipeline(pipeline *computedata.Pipeline)
	UnloadComputeBindings(group *computedata.BindGroup)

	Run(ctx context.Context, update func(time.Duration, InputState) (render.Frame, error)) error
	Capture(ctx context.Context) (*image.RGBA, error)
	UnloadTexture(source *texturedata.Texture)
	Close() error
}
