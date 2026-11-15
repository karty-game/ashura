//go:build !js

package wgpu

import (
	"testing"
	"unsafe"
)

// Offsets come from WGPUBindGroupLayoutEntry in the pinned v29 public C header.
func TestBindingLayoutMatchesNativeV29(t *testing.T) {
	var entry bindGroupLayoutEntryWire
	if unsafe.Offsetof(entry.Visibility) != 16 || unsafe.Offsetof(entry.BindingArraySize) != 24 ||
		unsafe.Offsetof(entry.Buffer) != 32 || unsafe.Offsetof(entry.Sampler) != 56 ||
		unsafe.Offsetof(entry.Texture) != 72 || unsafe.Offsetof(entry.StorageTexture) != 96 || unsafe.Sizeof(entry) != 120 {
		t.Fatal("native v29 bind-group layout ABI mismatch")
	}
}
