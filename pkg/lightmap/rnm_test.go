package lightmap_test

import (
	"encoding/binary"
	"math"
	"testing"

	"github.com/karty-game/ashura/internal/computedata"
	"github.com/karty-game/ashura/internal/lightmapdata"
	"github.com/karty-game/ashura/internal/lightmapfixture"
	"github.com/karty-game/ashura/pkg/compute"
	"github.com/karty-game/ashura/pkg/lightmap"
)

func TestRNMProgressiveSchedulingAndImmutableScene(t *testing.T) {
	t.Parallel()

	document := lightmapfixture.RNMDocument()
	document.Height = 2
	document.Receivers = append(document.Receivers, document.Receivers...)
	document.Frames = append(document.Frames, document.Frames...)

	document.Padding = append(document.Padding, document.Padding...)
	for i, value := range []uint32{4, 5, 5, math.MaxUint32} {
		binary.LittleEndian.PutUint32(document.Padding[16+i*4:], value)
	}

	encoded, err := lightmapdata.Encode(document)
	if err != nil {
		t.Fatal(err)
	}

	asset, err := lightmap.Decode(encoded)
	if err != nil {
		t.Fatal(err)
	}

	resources := &owner{}

	baker, err := lightmap.NewRNM(resources, asset)
	if err != nil {
		t.Fatal(err)
	}

	_ = resources.uploads()

	scene := lightmap.RNMScene{
		Points:        []lightmap.Point{{Position: [3]float32{0, 2, 0}, Color: [3]float32{1, .5, 0}, Intensity: 2, Range: 10}},
		RowsPerRecord: 1,
	}
	if err = baker.SetScene(scene); err != nil {
		t.Fatal(err)
	}

	if resources.uploads() != 1 {
		t.Fatal("settings update did not upload only point bytes")
	}

	if err = baker.Record(resources); err != nil {
		t.Fatal(err)
	}

	if !baker.Pending() || baker.Progress().Rows != 1 || resources.dispatches != 1 {
		t.Fatal("first row did not remain progressive")
	}

	_ = resources.uploads()

	if err = baker.SetScene(scene); err != nil {
		t.Fatal(err)
	}

	if resources.uploads() != 0 || baker.Progress().Rows != 1 {
		t.Fatal("unchanged lighting restarted work")
	}

	if err = baker.Record(resources); err != nil {
		t.Fatal(err)
	}

	if baker.Pending() || !baker.Progress().Complete || baker.Recordings() != 3 || resources.dispatches != 3 {
		t.Fatal("last row did not resolve exactly once")
	}

	_ = resources.uploads()
	if err = baker.Record(resources); err != nil || resources.uploads() != 0 || resources.dispatches != 3 {
		t.Fatal("completed bake did extra work")
	}

	scene.Points[0].Position[0]++
	if err = baker.SetScene(scene); err != nil {
		t.Fatal(err)
	}

	if !baker.Pending() || baker.Progress().Rows != 0 {
		t.Fatal("caller mutation was retained instead of copied")
	}

	scene.Samples = 65
	if err = baker.SetScene(scene); err == nil || baker.Progress().Rows != 0 {
		t.Fatal("invalid scene admitted or changed state")
	}
}

type recordingRNMOwner struct {
	owner

	entries []string
}

func (resources *recordingRNMOwner) Dispatch(pipeline *compute.Pipeline, group *compute.BindGroup, x, y, z uint32) error {
	resources.entries = append(resources.entries, computedata.Description(pipeline).EntryPoint)

	return resources.owner.Dispatch(pipeline, group, x, y, z)
}

func TestPreparedRNMSchedulesPatchBeforeRowsAndPublishesScale(t *testing.T) {
	t.Parallel()

	document := lightmapfixture.RNMDocument()
	document.GISamples = 64

	document.ReceiverGIIndex = make([]byte, 16)
	for index, value := range []uint32{0, 1, math.MaxUint32, math.MaxUint32} {
		binary.LittleEndian.PutUint32(document.ReceiverGIIndex[index*4:], value)
	}

	document.GIReceivers = make([]byte, 64)
	binary.LittleEndian.PutUint32(document.GIReceivers[4:], 1)
	binary.LittleEndian.PutUint32(document.GIReceivers[32:], 1)

	document.GIPatches = make([]byte, 48)
	for index, value := range []float32{0, 1, 0, 0, 0, -1, 0, 0, .5, 0, 0, 0} {
		binary.LittleEndian.PutUint32(document.GIPatches[index*4:], math.Float32bits(value))
	}

	document.GITransfers = make([]byte, 16)
	for index := range 3 {
		binary.LittleEndian.PutUint32(document.GITransfers[4+index*4:], math.Float32bits(.2))
	}

	encoded, err := lightmapdata.Encode(document)
	if err != nil {
		t.Fatal(err)
	}

	asset, err := lightmap.Decode(encoded)
	if err != nil {
		t.Fatal(err)
	}

	resources := &recordingRNMOwner{}

	baker, err := lightmap.NewRNM(resources, asset)
	if err != nil {
		t.Fatal(err)
	}

	scene := lightmap.RNMScene{
		Bounces: 1,
		Points:  []lightmap.Point{{Position: [3]float32{0, 2, 0}, Color: [3]float32{1, 1, 1}, Intensity: 2, Range: 10}},
	}
	if err = baker.SetScene(scene); err != nil {
		t.Fatal(err)
	}

	if baker.Lightmap().Scale != 1 {
		t.Fatal("new scale published before atlas resolve")
	}

	if err = baker.Record(resources); err != nil {
		t.Fatal(err)
	}

	expected := []string{"bake_rnm_patches", "bake_rnm", "resolve_rnm"}
	for index, entry := range expected {
		if len(resources.entries) <= index || resources.entries[index] != entry {
			t.Fatalf("prepared dispatch ordering: %v", resources.entries)
		}
	}

	if resources.dispatches != 3 || baker.Recordings() != 3 || baker.Pending() || baker.Lightmap().Scale <= 2 {
		t.Fatal("prepared patch, receiver and resolve dispatch schedule or scale publication incorrect")
	}
}
