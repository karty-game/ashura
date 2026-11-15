//go:build !js

package wgpu

import (
	"testing"

	"github.com/gogpu/gputypes"
)

// Expected values come from the pinned v29 webgpu.h, not from gputypes.
// gputypes v0.5 renumbered these enums to make their zero values defaults.
func TestPrimitiveEnumsMatchNativeV29(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		topology gputypes.PrimitiveTopology
		native   uint32
	}{
		{gputypes.PrimitiveTopologyTriangleList, 4},
		{gputypes.PrimitiveTopologyPointList, 1},
		{gputypes.PrimitiveTopologyLineList, 2},
		{gputypes.PrimitiveTopologyLineStrip, 3},
		{gputypes.PrimitiveTopologyTriangleStrip, 5},
	} {
		if got := toWGPUPrimitiveTopology(test.topology); got != test.native {
			t.Fatalf("topology %v: native %d, want %d", test.topology, got, test.native)
		}
	}

	for _, test := range []struct {
		face   gputypes.FrontFace
		native uint32
	}{
		{gputypes.FrontFaceCCW, 1},
		{gputypes.FrontFaceCW, 2},
	} {
		if got := toWGPUFrontFace(test.face); got != test.native {
			t.Fatalf("front face %v: native %d, want %d", test.face, got, test.native)
		}
	}

	for _, test := range []struct {
		mode   gputypes.CullMode
		native uint32
	}{
		{gputypes.CullModeNone, 1},
		{gputypes.CullModeFront, 2},
		{gputypes.CullModeBack, 3},
	} {
		if got := toWGPUCullMode(test.mode); got != test.native {
			t.Fatalf("cull mode %v: native %d, want %d", test.mode, got, test.native)
		}
	}
}
