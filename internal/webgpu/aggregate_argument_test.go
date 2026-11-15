//go:build linux || darwin

package wgpu

import (
	"testing"
	"unsafe"
)

func TestAggregateArgumentABI(t *testing.T) {
	for _, test := range []struct {
		name  string
		index int
		size  uintptr
	}{
		{"wgpuInstanceRequestAdapter", 2, 40}, {"wgpuAdapterRequestDevice", 2, 40}, {"wgpuBufferMapAsync", 4, 40},
		{"wgpuDevicePopErrorScope", 1, 40}, {"wgpuAdapterInfoFreeMembers", 0, 96}, {"wgpuSupportedFeaturesFreeMembers", 0, 16},
	} {
		index, descriptor := aggregateArgument(test.name)
		if index != test.index || descriptor == nil || descriptor.Size != test.size {
			t.Fatal(test, index, descriptor)
		}
	}
}
func TestNativeStringsOwnTheirBytes(t *testing.T) {
	bytes := []byte("native adapter")
	value := stringViewToString(StringView{Data: uintptr(unsafe.Pointer(&bytes[0])), Length: uintptr(len(bytes))})
	bytes[0] = 'X'
	if value != "native adapter" {
		t.Fatal("returned string aliases native allocation", value)
	}
}
