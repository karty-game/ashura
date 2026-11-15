//go:build !js

package wgpu

import (
	"testing"
	"unsafe"

	"github.com/gogpu/gputypes"
)

// These offsets and array strides follow WGPUVertexAttribute and
// WGPUVertexBufferLayout in the pinned wgpu-native v29 public header.
func TestVertexInputMatchesNativeV29(t *testing.T) {
	t.Parallel()

	var attribute vertexAttributeWire
	if unsafe.Offsetof(attribute.NextInChain) != 0 || unsafe.Offsetof(attribute.Format) != 8 ||
		unsafe.Offsetof(attribute.Offset) != 16 || unsafe.Offsetof(attribute.ShaderLocation) != 24 ||
		unsafe.Sizeof(attribute) != 32 {
		t.Fatal("native v29 vertex attribute ABI mismatch")
	}

	var buffer vertexBufferLayoutWire
	if unsafe.Offsetof(buffer.NextInChain) != 0 || unsafe.Offsetof(buffer.StepMode) != 8 ||
		unsafe.Offsetof(buffer.ArrayStride) != 16 || unsafe.Offsetof(buffer.AttributeCount) != 24 ||
		unsafe.Offsetof(buffer.Attributes) != 32 || unsafe.Sizeof(buffer) != 40 {
		t.Fatal("native v29 vertex buffer layout ABI mismatch")
	}
}

func TestModelVertexFormatsMatchNativeV29(t *testing.T) {
	t.Parallel()

	// The public Go enum has different values from WGPUVertexFormat. These are
	// the v29 header values used by model, instance, sprite and UI buffers.
	for _, test := range []struct {
		format gputypes.VertexFormat
		native uint32
	}{
		{gputypes.VertexFormatFloat32x2, 0x1d},
		{gputypes.VertexFormatFloat32x3, 0x1e},
		{gputypes.VertexFormatFloat32x4, 0x1f},
		{gputypes.VertexFormatUnorm8x4, 0x09},
	} {
		if got := toWGPUVertexFormat(test.format); got != test.native {
			t.Fatalf("vertex format %v: native value %#x, want %#x", test.format, got, test.native)
		}
	}
}
