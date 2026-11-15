//go:build !js

package lightmapper

import (
	"bytes"
	"errors"
	"math"
	"reflect"
	"testing"
)

func TestReconstructionAutomaticHaloKeepsReferenceAndPacket(t *testing.T) {
	t.Parallel()

	receivers, halo, edges, options := reconstructionGraphFixture()

	ids := make([]uint64, len(receivers))
	for index, receiver := range receivers {
		ids[index] = receiver.ID
	}
	//nolint:gosec // G115: reconstructionGraphFixture supplies exactly six halo records.
	options.MaximumHalo = uint32(len(halo))

	explicit, err := ReconstructLinear(ids, halo, edges, options)
	if err != nil {
		t.Fatal(err)
	}

	packet, err := PackGPUReconstruction(ids, halo, edges, options)
	if err != nil {
		t.Fatal(err)
	}

	options.MaximumHalo = 0

	automatic, err := ReconstructLinear(ids, halo, edges, options)
	if err != nil || !reflect.DeepEqual(automatic, explicit) {
		t.Fatal("automatic halo changed CPU reference", err)
	}

	autoPacket, err := PackGPUReconstruction(ids, halo, edges, options)
	if err != nil || !bytes.Equal(autoPacket.Data, packet.Data) {
		t.Fatal("automatic halo changed admitted compute packet", err)
	}

	if reconstructionHaloFits(math.MaxUint32+1, options) {
		t.Fatal("automatic scheduling removed the existing 32-bit wire count bound")
	}
	//nolint:gosec // G115: reconstructionGraphFixture supplies exactly six halo records.
	options.MaximumHalo = uint32(len(halo) - 1)
	if _, err = ReconstructLinear(ids, halo, edges, options); err == nil {
		t.Fatal("explicit CPU halo ceiling ignored")
	}

	if _, err = PackGPUReconstruction(ids, halo, edges, options); err == nil {
		t.Fatal("explicit GPU halo ceiling ignored")
	}
}

func TestReconstructionAutomaticHaloRetainsStorageAndProgressBounds(t *testing.T) {
	t.Parallel()

	receivers, halo, edges, options := reconstructionGraphFixture()

	graph, err := newReconstructionGraph(receivers, halo, edges)
	if err != nil {
		t.Fatal(err)
	}

	options.MaximumHalo = 0

	device := GPUDevice{MaxBufferSize: 1 << 30, MaxStorageBufferBindingSize: 1 << 30, MaxStorageBuffersPerShaderStage: 4}
	if _, _, err = graph.nextBatch(0, 256, options, device, 256); !errors.Is(err, ErrBounds) {
		t.Fatal("automatic scheduling ignored complete live memory", err)
	}

	end, selected, err := graph.nextBatch(0, 256, options, device, 1<<30)
	if err != nil || end != len(receivers) || len(selected) != len(receivers) {
		t.Fatal("failed admission poisoned uncapped complete roster", end, err)
	}

	device.MaxStorageBufferBindingSize = 1024
	device.MaxBufferSize = 1024
	count := 0

	for first := 0; first < len(receivers); {
		last, selected, err := graph.nextBatch(first, 256, options, device, 1<<30)
		if err != nil || last <= first {
			t.Fatal("automatic storage partition made no progress", err)
		}

		packet := graph.pack(first, last, selected, options)
		if uint64(len(packet.Data)) > device.MaxStorageBufferBindingSize {
			t.Fatal("automatic halo exceeded negotiated storage")
		}

		count++
		first = last
	}

	if count < 2 {
		t.Fatal("fixture did not exercise negotiated partitioning")
	}
}
