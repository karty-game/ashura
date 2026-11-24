package engine

import (
	"encoding/binary"
	"errors"
	"testing"

	"github.com/karty-game/ashura/internal/decaldata"
	"github.com/karty-game/ashura/internal/render"
)

func TestAuthoredDecalOrderTracksResidency(t *testing.T) {
	t.Parallel()

	newBatch := func(orders ...int32) *PreparedDecals {
		t.Helper()

		marks := make([]decaldata.Placement, len(orders))
		for index, order := range orders {
			marks[index] = decaldata.Placement{
				Normal: decaldata.Vector{0, 1, 0}, Size: decaldata.Vector{1, 1, 1},
				Max: [2]float32{1, 1}, Tint: [4]float32{1, 1, 1, 1}, Layers: 1, Order: order,
			}
		}

		data, err := decaldata.Encode(marks)
		if err != nil {
			t.Fatal(err)
		}

		prepared, err := DecodePreparedDecals(data)
		if err != nil {
			t.Fatal(err)
		}

		return prepared
	}

	world := NewWorld()
	atlas := testDecalTexture(t, NewApp(AppConfig{}))

	first, err := world.MountDecals(newBatch(9, -8, 9), atlas)
	if err != nil {
		t.Fatal(err)
	}

	second, err := world.MountDisabledDecals(newBatch(-8, 2), atlas)
	if err != nil {
		t.Fatal(err)
	}

	var uploads render.StaticDecalUploads

	write := func([]byte) error { return nil }
	check := func(changed bool, want ...uint32) {
		t.Helper()

		frame, err := world.decals.renderFrame()
		if err != nil || frame.Static == nil {
			t.Fatal("expected a static decal frame", err)
		}

		wrote, err := uploads.Order(frame.Static, write)
		if err != nil || wrote != changed {
			t.Fatalf("rank table upload = %t, want %t, err = %v", wrote, changed, err)
		}

		table := frame.Static.Order.Bytes[:]
		for rank, slot := range want {
			byRank := binary.LittleEndian.Uint32(table[4*rank:]) >> render.StaticDecalSlotBits
			bySlot := binary.LittleEndian.Uint32(table[4*slot:]) & render.StaticDecalSlotMask

			if byRank != slot || int(bySlot) != rank {
				t.Fatalf("packed order at rank %d maps slot %d to rank %d, want slot %d", rank, byRank, bySlot, slot)
			}
		}
	}

	check(true, 1, 32, 33, 0, 2)

	if err := second.SetEnabled(1, true); err != nil {
		t.Fatal(err)
	}

	check(false, 1, 32, 33, 0, 2)
	first.Remove()
	check(true, 32, 33)

	if _, err := world.MountDecals(newBatch(-300, 9), atlas); err != nil {
		t.Fatal(err)
	}

	check(true, 0, 32, 33, 1)
	check(false, 0, 32, 33, 1)
}

func TestAuthoredDecalsHaveIndependentResidencyAndDirtyRanges(t *testing.T) {
	t.Parallel()

	values := make([]decaldata.Placement, decaldata.MaxBatch)
	for index := range values {
		values[index] = decaldata.Placement{
			Position: decaldata.Vector{float32(index), 0, 0},
			Normal:   decaldata.Vector{0, 1, 0}, Size: decaldata.Vector{1, 1, 1},
			Max: [2]float32{1, 1}, Tint: [4]float32{1, 1, 1, 1}, Layers: 1,
		}
	}

	encoded, err := decaldata.Encode(values)
	if err != nil {
		t.Fatal(err)
	}

	batch, err := DecodePreparedDecals(encoded)
	if err != nil {
		t.Fatal(err)
	}

	encoded[len(encoded)-1] = 1
	if _, err := DecodePreparedDecals(encoded); !errors.Is(err, decaldata.ErrBatch) {
		t.Fatal("noncanonical record should fail", err)
	}

	app, world := NewApp(AppConfig{}), NewWorld()
	app.scene = world
	atlas := testDecalTexture(t, app)
	count := decaldata.MaxStatic / decaldata.MaxBatch

	mounted := make([]*DecalPlacement, count)
	for index := range mounted {
		mounted[index], err = world.MountDecals(batch, atlas)
		if err != nil {
			t.Fatal(err)
		}
	}

	if _, err := world.MountDecals(batch, atlas); !errors.Is(err, errStaticDecalCapacity) {
		t.Fatal("large batch should hit authored capacity", err)
	}

	world.Add(DecalProjector{
		Image: DecalImage{Texture: atlas}, Normal: Vector3Up(), Size: Vector3One(),
		Tint: ColorWhite(), ReceiverMask: 1,
	})

	frame, err := makeFrame(world, 16, 9)
	if err != nil || frame.Decals.Count != 1 || frame.Decals.Static.End != decaldata.MaxStatic {
		t.Fatalf("combined frame = %+v: %v", frame.Decals, err)
	}

	var (
		uploads  render.StaticDecalUploads
		uploaded int
	)

	write := func(_ int, data []byte) error {
		uploaded += len(data)

		return t.Context().Err()
	}
	if _, err := uploads.Update(frame.Decals.Static, write); err != nil || uploaded != render.StaticDecalBytes {
		t.Fatalf("initial upload %d: %v", uploaded, err)
	}

	uploaded = 0

	if err := mounted[2].SetEnabled(75, false); err != nil {
		t.Fatal(err)
	}

	if _, err := uploads.Update(frame.Decals.Static, write); err != nil || uploaded != decaldata.RecordBytes {
		t.Fatalf("toggle uploaded %d: %v", uploaded, err)
	}

	uploaded = 0
	if _, err := uploads.Update(frame.Decals.Static, write); err != nil || uploaded != 0 {
		t.Fatalf("unchanged frame uploaded %d: %v", uploaded, err)
	}

	if err := app.UnloadTexture(atlas); !errors.Is(err, ErrInUse) {
		t.Fatal("mounted authored decals should retain their atlas", err)
	}

	mounted[1].Remove()
	mounted[1].Remove()

	if _, err := world.MountDecals(batch, atlas); err != nil {
		t.Fatal("retired pages must be reusable", err)
	}
}

func TestDormantDecalBatchPresentsOnlyEnabledRecords(t *testing.T) {
	t.Parallel()

	const lastCell = render.StaticDecalPageRecords

	values := make([]decaldata.Placement, lastCell+1)
	for index := range values {
		values[index] = decaldata.Placement{
			Normal: decaldata.Vector{0, 1, 0}, Size: decaldata.Vector{1, 1, 1},
			Max: [2]float32{1, 1}, Tint: [4]float32{1, 1, 1, 1}, Layers: 1,
		}
	}

	data, err := decaldata.Encode(values)
	if err != nil {
		t.Fatal(err)
	}

	batch, err := DecodePreparedDecals(data)
	if err != nil {
		t.Fatal(err)
	}

	app, world := NewApp(AppConfig{}), NewWorld()

	mounted, err := world.MountDisabledDecals(batch, testDecalTexture(t, app))
	if err != nil {
		t.Fatal(err)
	}

	frame := &world.decals.static.frame
	if frame.End != 0 || frame.Revision != 0 {
		t.Fatal("reserving a dormant batch presented records")
	}

	if err := mounted.SetEnabled(lastCell, true); err != nil {
		t.Fatal(err)
	}

	var (
		uploads render.StaticDecalUploads
		written int
	)

	_, err = uploads.Update(frame, func(_ int, record []byte) error {
		written += len(record)

		return t.Context().Err()
	})
	if err != nil || written != decaldata.RecordBytes {
		t.Fatalf("enabling one cell wrote %d bytes: %v", written, err)
	}

	if err := mounted.SetEnabled(lastCell, false); err != nil {
		t.Fatal(err)
	}

	if frame.End != 0 {
		t.Fatal("hiding the last cell left active pages")
	}

	mounted.Remove()
}
