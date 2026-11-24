package render

import (
	"encoding/binary"
	"errors"
	"io"
	"slices"
	"testing"
)

func TestDecalUploadsUpdateChangedSlotsOnly(t *testing.T) {
	t.Parallel()

	const secondOffset = DecalHeaderBytes + DecalRecordBytes

	frame := &DecalFrame{Count: 2}
	frame.Records[0][0], frame.Records[1][0] = 11, 22

	var (
		tracker    DecalUploads
		offsets    []int
		count      uint32
		failOffset = -1
	)

	write := func(offset int, data []byte) error {
		if offset == failOffset {
			return io.ErrShortWrite
		}

		offsets = append(offsets, offset)
		if offset == 0 {
			count = binary.LittleEndian.Uint32(data)
		}

		return nil
	}
	check := func(value *DecalFrame, expectedCount uint32, expected ...int) {
		t.Helper()

		offsets = offsets[:0]

		if err := tracker.Update(value, write); err != nil {
			t.Fatal(err)
		}

		if !slices.Equal(offsets, expected) || count != expectedCount {
			t.Fatalf("writes %v, count %d, want %v and %d", offsets, count, expected, expectedCount)
		}
	}
	check(frame, 2, 0, DecalHeaderBytes, secondOffset)
	check(frame, 2)

	frame.Revision++
	frame.Records[1][0] = 42
	failOffset = secondOffset

	if err := tracker.Update(frame, write); !errors.Is(err, io.ErrShortWrite) {
		t.Fatal("write failure was not reported", err)
	}

	failOffset = -1

	check(frame, 2, secondOffset)
	check(nil, 0, 0)
	check(frame, 2, 0)
}
