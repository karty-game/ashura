package render

import "github.com/karty-game/ashura/internal/decaldata"

const (
	StaticDecalPageRecords = 32
	StaticDecalPageBytes   = StaticDecalPageRecords * decaldata.RecordBytes
	StaticDecalPages       = decaldata.MaxStatic / StaticDecalPageRecords
	StaticDecalBytes       = decaldata.MaxStatic * decaldata.RecordBytes
	DecalTileAxis          = 16
	DecalTileCount         = DecalTileAxis * DecalTileAxis
	DecalTileLimit         = 64
	DecalTileBytes         = DecalTileCount * (DecalTileLimit + 1) * 4
	decalMaskWordBits      = 32
	DecalTileMaskBytes     = DecalTileCount * (decaldata.MaxStatic / decalMaskWordBits) * 4
	StaticDecalSlotBits    = 13
	StaticDecalSlotMask    = (1 << StaticDecalSlotBits) - 1
	StaticDecalOrderBytes  = decaldata.MaxStatic * 4
	DecalTileStorageBytes  = DecalTileMaskBytes + StaticDecalOrderBytes
	DecalViewBytes         = 80
)

type StaticDecalFrame struct {
	Pages    [StaticDecalPages][StaticDecalPageBytes]byte
	Versions [StaticDecalPages]uint64
	Records  [StaticDecalPages][StaticDecalPageRecords]uint64
	Revision uint64
	End      int
	Order    StaticDecalOrder
}

// StaticDecalOrder stores both slot-to-rank and rank-to-slot in each word.
// The low 13 bits contain the rank and the next 13 bits contain the slot.
type StaticDecalOrder struct {
	Bytes    [StaticDecalOrderBytes]byte
	Revision uint64
}

type StaticDecalUploads struct {
	frame         *StaticDecalFrame
	revision      uint64
	versions      [StaticDecalPages]uint64
	view          [DecalViewBytes]byte
	order         *StaticDecalOrder
	orderRevision uint64
}

func (state *StaticDecalUploads) Update(frame *StaticDecalFrame, write func(int, []byte) error) (bool, error) {
	if frame == nil {
		return false, nil
	}

	if state.frame == frame && state.revision == frame.Revision {
		return false, nil
	}

	changedWorld := state.frame != nil && state.frame != frame
	for index, revision := range frame.Versions {
		if !changedWorld && state.versions[index] == revision || changedWorld && state.versions[index] == 0 && revision == 0 {
			continue
		}

		if err := state.writePage(frame, index, changedWorld, write); err != nil {
			return false, err
		}

		state.versions[index] = revision
	}

	state.frame, state.revision = frame, frame.Revision

	return true, nil
}

// Order uploads the rank table only when the resident batches change.
func (state *StaticDecalUploads) Order(frame *StaticDecalFrame, write func([]byte) error) (bool, error) {
	if frame == nil || state.order == &frame.Order && state.orderRevision == frame.Order.Revision {
		return false, nil
	}

	if err := write(frame.Order.Bytes[:]); err != nil {
		return false, err
	}

	state.order, state.orderRevision = &frame.Order, frame.Order.Revision

	return true, nil
}

// View encodes camera inputs for culling. A stationary camera leaves this
// buffer untouched. A change in width/height follows swapchain resizing.
func (state *StaticDecalUploads) View(frame Frame, width, height int, write func([]byte) error) (bool, error) {
	var values [DecalViewBytes / 4]float32
	if frame.Decals != nil && frame.Decals.Static != nil {
		copy(values[:16], frame.ViewProjection[:])
		values[16], values[17] = float32(max(1, width)), float32(max(1, height))
		values[18] = float32(frame.Decals.Static.End)
	}

	var data [DecalViewBytes]byte
	encodeUniform(data[:], values[:])

	if data == state.view {
		return false, nil
	}

	if err := write(data[:]); err != nil {
		return false, err
	}

	state.view = data

	return true, nil
}

func (state *StaticDecalUploads) writePage(
	frame *StaticDecalFrame, page int, reset bool, write func(int, []byte) error,
) error {
	offset := page * StaticDecalPageBytes
	if reset {
		return write(offset, frame.Pages[page][:])
	}

	previous := state.versions[page]

	records := frame.Records[page]
	for first := 0; first < len(records); {
		if records[first] <= previous {
			first++

			continue
		}

		end := first + 1
		for end < len(records) && records[end] > previous {
			end++
		}

		beginByte, endByte := first*decaldata.RecordBytes, end*decaldata.RecordBytes
		if err := write(offset+beginByte, frame.Pages[page][beginByte:endByte]); err != nil {
			return err
		}

		first = end
	}

	return nil
}
