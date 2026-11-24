package engine

import (
	"encoding/binary"
	"errors"
	"slices"

	"github.com/karty-game/ashura/internal/decaldata"
	"github.com/karty-game/ashura/internal/render"
	"github.com/karty-game/ashura/pkg/texture"
)

var errStaticDecalCapacity = errors.New("engine: prepared decal residency exhausted")

// PreparedDecals owns immutable GPU-ready projector records with world-space placement.
type PreparedDecals struct{ records []byte }

// DecodePreparedDecals admits a bounded, versioned offline decal batch.
func DecodePreparedDecals(data []byte) (*PreparedDecals, error) {
	records, err := decaldata.Decode(data)
	if err != nil {
		return nil, err
	}

	return &PreparedDecals{records: records}, nil
}

// Count returns the number of independently addressable prepared projectors.
func (prepared *PreparedDecals) Count() int {
	if prepared == nil {
		return 0
	}

	return len(prepared.records) / decaldata.RecordBytes
}

type staticDecals struct {
	frame      render.StaticDecalFrame
	used       [render.StaticDecalPages]bool
	live       [render.StaticDecalPages]int
	atlas      *texture.Texture
	mounted    int
	enabled    int
	orderKeys  []uint64
	orderDirty bool
}

func (state *worldDecals) renderFrame() (*render.DecalFrame, error) {
	if state.static == nil || state.static.mounted == 0 {
		state.frame.Atlas, state.frame.Static = state.liveAtlas, nil
		if state.frame.Count == 0 {
			return nil, nil //nolint:nilnil // Absence of an active decal frame is normal.
		}

		return &state.frame, nil
	}

	if state.liveAtlas != nil && state.liveAtlas != state.static.atlas {
		return nil, errDecalAtlas
	}

	state.frame.Atlas, state.frame.Static = state.static.atlas, nil
	if state.static.enabled != 0 {
		state.static.prepareOrder()
		state.frame.Static = &state.static.frame
	}

	return &state.frame, nil
}

// DecalPlacement owns one resident prepared batch. Its methods belong to the App loop.
type DecalPlacement struct{ resident *mountedDecals }

type mountedDecals struct {
	batch        *PreparedDecals
	state        *staticDecals
	first, pages int
}

// MountDecals reserves one prepared batch until Remove. All active projectors
// in a World share an immutable sRGB atlas. Mount performs no projector math.
func (world *World) MountDecals(batch *PreparedDecals, atlas *texture.Texture) (*DecalPlacement, error) {
	return world.mountDecals(batch, atlas, true)
}

// MountDisabledDecals reserves a prepared batch with every projector disabled.
// SetEnabled publishes individual immutable records as they become visible.
func (world *World) MountDisabledDecals(batch *PreparedDecals, atlas *texture.Texture) (*DecalPlacement, error) {
	return world.mountDecals(batch, atlas, false)
}

func (world *World) mountDecals(batch *PreparedDecals, atlas *texture.Texture, enabled bool) (*DecalPlacement, error) {
	if world == nil || batch.Count() == 0 {
		return nil, decaldata.ErrBatch
	}

	if !validDecalAtlas(atlas) {
		return nil, errDecalAtlas
	}

	if world.decals == nil {
		world.decals = &worldDecals{}
	}

	worldDecals := world.decals
	if worldDecals.static == nil {
		worldDecals.static = &staticDecals{orderKeys: make([]uint64, 0, decaldata.MaxStatic)}
	}

	state := worldDecals.static
	if state.atlas != nil && state.atlas != atlas {
		return nil, errDecalAtlas
	}

	const pageRecords = render.StaticDecalPageRecords

	pages := (batch.Count() + pageRecords - 1) / pageRecords

	first := state.reserve(pages)
	if first < 0 {
		return nil, errStaticDecalCapacity
	}

	state.atlas = atlas
	state.mounted++
	state.addOrder(first*pageRecords, batch)

	placement := &mountedDecals{batch: batch, state: state, first: first, pages: pages}
	if enabled {
		placement.showBatch()
	}

	return &DecalPlacement{resident: placement}, nil
}

func (placement *mountedDecals) showBatch() {
	state, batch := placement.state, placement.batch
	state.enabled += batch.Count()
	state.frame.Revision++

	for page := range placement.pages {
		index := placement.first + page
		offset := page * render.StaticDecalPageBytes
		data := batch.records[offset:min(offset+render.StaticDecalPageBytes, len(batch.records))]
		copy(state.frame.Pages[index][:], data)

		state.frame.Versions[index] = state.frame.Revision

		state.live[index] = len(data) / decaldata.RecordBytes
		for record := range state.live[index] {
			state.frame.Records[index][record] = state.frame.Revision
		}
	}

	state.frame.End = max(state.frame.End, (placement.first+placement.pages)*render.StaticDecalPageRecords)
}

func (state *staticDecals) reserve(pages int) int {
	var run int

	for index, used := range state.used {
		if used {
			run = 0

			continue
		}

		run++
		if run == pages {
			first := index + 1 - pages
			for page := first; page <= index; page++ {
				state.used[page] = true
			}

			return first
		}
	}

	return -1
}

// SetEnabled controls the projector at its authored index.
func (handle *DecalPlacement) SetEnabled(index int, enabled bool) error {
	if handle == nil || handle.resident == nil || handle.resident.state == nil {
		return ErrClosed
	}

	placement := handle.resident
	if index < 0 || index >= placement.batch.Count() {
		return decaldata.ErrBatch
	}

	state := placement.state
	page := placement.first + index/render.StaticDecalPageRecords
	offset := index % render.StaticDecalPageRecords * decaldata.RecordBytes
	record := state.frame.Pages[page][offset : offset+decaldata.RecordBytes]

	active := binary.LittleEndian.Uint32(record[decaldata.MaskOffset:]) != 0
	if active == enabled {
		return nil
	}

	if enabled {
		start := index * decaldata.RecordBytes
		copy(record, placement.batch.records[start:start+decaldata.RecordBytes])

		state.enabled++
		state.live[page]++
		state.frame.End = max(state.frame.End, (page+1)*render.StaticDecalPageRecords)
	} else {
		clear(record)

		state.enabled--
		state.live[page]--
		state.trim()
	}

	state.frame.Revision++
	state.frame.Versions[page] = state.frame.Revision
	state.frame.Records[page][index%render.StaticDecalPageRecords] = state.frame.Revision

	return nil
}

// Remove retires the batch and releases its atlas reference. It is idempotent.
func (handle *DecalPlacement) Remove() {
	if handle == nil || handle.resident == nil || handle.resident.state == nil {
		return
	}

	placement := handle.resident
	state := placement.state
	state.frame.Revision++

	for page := placement.first; page < placement.first+placement.pages; page++ {
		for index := range render.StaticDecalPageRecords {
			offset := index*decaldata.RecordBytes + decaldata.MaskOffset

			if binary.LittleEndian.Uint32(state.frame.Pages[page][offset:]) != 0 {
				state.enabled--
				state.frame.Records[page][index] = state.frame.Revision
			}
		}

		clear(state.frame.Pages[page][:])
		state.frame.Versions[page], state.used[page] = state.frame.Revision, false
		state.live[page] = 0
	}

	state.trim()
	state.removeOrder(placement.first*render.StaticDecalPageRecords, placement.batch.Count())

	state.mounted--
	if state.mounted == 0 {
		state.atlas = nil
	}

	*placement = mountedDecals{}
}

// addOrder reads the signed order once for each newly resident slot. Disabled
// slots keep their order so toggling a mark only changes its projector record.
func (state *staticDecals) addOrder(first int, batch *PreparedDecals) {
	const signBit = 1 << 31

	for index := range batch.Count() {
		offset := index*decaldata.RecordBytes + decaldata.OrderOffset
		order := binary.LittleEndian.Uint32(batch.records[offset:]) ^ signBit
		slot := uint64(first + index) //nolint:gosec // Slots stay within the reserved page range.
		state.orderKeys = append(state.orderKeys, uint64(order)<<render.StaticDecalSlotBits|slot)
	}

	state.orderDirty = true
}

func (state *staticDecals) removeOrder(first, count int) {
	state.orderKeys = slices.DeleteFunc(state.orderKeys, func(key uint64) bool {
		slot := int(key & render.StaticDecalSlotMask)

		return first <= slot && slot < first+count
	})
	state.orderDirty = true
}

func (state *staticDecals) prepareOrder() {
	if !state.orderDirty {
		return
	}

	slices.Sort(state.orderKeys)

	order := &state.frame.Order
	clear(order.Bytes[:])

	const wordBytes = 4

	for rank, key := range state.orderKeys {
		slot := int(key & render.StaticDecalSlotMask)
		slotWord := order.Bytes[slot*wordBytes:]
		rankWord := order.Bytes[rank*wordBytes:]
		binary.LittleEndian.PutUint32(slotWord, binary.LittleEndian.Uint32(slotWord)|uint32(rank))
		binary.LittleEndian.PutUint32(rankWord, binary.LittleEndian.Uint32(rankWord)|uint32(slot)<<render.StaticDecalSlotBits)
	}

	order.Revision++
	state.orderDirty = false
}

func (state *staticDecals) trim() {
	const pageRecords = render.StaticDecalPageRecords
	for state.frame.End > 0 && state.live[state.frame.End/pageRecords-1] == 0 {
		state.frame.End -= pageRecords
	}
}
