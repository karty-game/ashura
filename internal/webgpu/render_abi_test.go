//go:build !js

package wgpu

import (
	"testing"
	"unsafe"
)

// Offsets and sizes follow the pinned wgpu-native v29 public header. In
// particular, both attachment structures start with nextInChain.
func TestRenderPassAttachmentsMatchNativeV29(t *testing.T) {
	t.Parallel()

	var depth renderPassDepthStencilAttachment
	if unsafe.Offsetof(depth.nextInChain) != 0 || unsafe.Offsetof(depth.view) != 8 ||
		unsafe.Offsetof(depth.depthLoadOp) != 16 || unsafe.Offsetof(depth.depthStoreOp) != 20 ||
		unsafe.Offsetof(depth.depthClearValue) != 24 || unsafe.Offsetof(depth.depthReadOnly) != 28 ||
		unsafe.Offsetof(depth.stencilLoadOp) != 32 || unsafe.Offsetof(depth.stencilStoreOp) != 36 ||
		unsafe.Offsetof(depth.stencilClearValue) != 40 || unsafe.Offsetof(depth.stencilReadOnly) != 44 ||
		unsafe.Sizeof(depth) != 48 {
		t.Fatal("native v29 render-pass depth attachment ABI mismatch")
	}

	var color renderPassColorAttachment
	if unsafe.Offsetof(color.nextInChain) != 0 || unsafe.Offsetof(color.view) != 8 ||
		unsafe.Offsetof(color.depthSlice) != 16 || unsafe.Offsetof(color.resolveTarget) != 24 ||
		unsafe.Offsetof(color.loadOp) != 32 || unsafe.Offsetof(color.storeOp) != 36 ||
		unsafe.Offsetof(color.clearValue) != 40 || unsafe.Sizeof(color) != 72 {
		t.Fatal("native v29 render-pass color attachment ABI mismatch")
	}
}

func TestRenderPassDescriptorMatchesNativeV29(t *testing.T) {
	t.Parallel()

	var pass renderPassDescriptor
	if unsafe.Offsetof(pass.nextInChain) != 0 || unsafe.Offsetof(pass.label) != 8 ||
		unsafe.Offsetof(pass.colorAttachmentCount) != 24 || unsafe.Offsetof(pass.colorAttachments) != 32 ||
		unsafe.Offsetof(pass.depthStencilAttachment) != 40 || unsafe.Offsetof(pass.occlusionQuerySet) != 48 ||
		unsafe.Offsetof(pass.timestampWrites) != 56 || unsafe.Sizeof(pass) != 64 {
		t.Fatal("native v29 render-pass descriptor ABI mismatch")
	}

	var timestamps passTimestampWrites
	if unsafe.Offsetof(timestamps.nextInChain) != 0 || unsafe.Offsetof(timestamps.querySet) != 8 ||
		unsafe.Offsetof(timestamps.beginningOfPassWriteIndex) != 16 ||
		unsafe.Offsetof(timestamps.endOfPassWriteIndex) != 20 || unsafe.Sizeof(timestamps) != 24 {
		t.Fatal("native v29 pass timestamps ABI mismatch")
	}

	var encoder commandEncoderDescriptorWire
	var commands CommandBufferDescriptor
	if unsafe.Offsetof(encoder.NextInChain) != 0 || unsafe.Offsetof(encoder.Label) != 8 || unsafe.Sizeof(encoder) != 24 ||
		unsafe.Offsetof(commands.NextInChain) != 0 || unsafe.Offsetof(commands.Label) != 8 || unsafe.Sizeof(commands) != 24 {
		t.Fatal("native v29 command encoder/buffer descriptor ABI mismatch")
	}
}
