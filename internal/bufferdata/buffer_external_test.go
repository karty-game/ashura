package bufferdata_test

import (
	"bytes"
	"testing"

	"github.com/karty-game/ashura/internal/bufferdata"
)

type dirtyFixture struct{ calls int }

func (owner *dirtyFixture) MarkBufferDirty(*bufferdata.Buffer) { owner.calls++ }

func TestBufferWritesPreserveGPUOwnedGaps(t *testing.T) {
	t.Parallel()

	const capacity = 16

	owner := &dirtyFixture{}
	initial := bytes.Repeat([]byte{1}, capacity)

	buffer, err := bufferdata.Create(owner, bufferdata.Desc{Usage: bufferdata.Storage, Data: initial})
	if err != nil {
		t.Fatal(err)
	}

	initial[0] = 9

	pending := bufferdata.Pending(buffer, nil)
	if len(pending) != 1 || pending[0].Data[0] != 1 {
		t.Fatal("initial buffer data was not copied")
	}

	const stride, rightOffset, disjointUploads, expectedNotifications = 4, 12, 2, 2

	left := []byte{2, 2, 2, 2}
	right := []byte{3, 3, 3, 3}

	if err := buffer.Write(0, left); err != nil {
		t.Fatal(err)
	}

	if err := buffer.Write(rightOffset, right); err != nil {
		t.Fatal(err)
	}

	// Invalid writes must not produce uploads or modify the accepted writes.
	for _, fixture := range []struct {
		offset uint64
		data   []byte
	}{
		{offset: 1, data: left},
		{offset: capacity, data: left},
		{offset: ^uint64(0), data: left},
		{data: left[:stride-1]},
	} {
		if err := buffer.Write(fixture.offset, fixture.data); err == nil {
			t.Fatal("accepted an unaligned or out-of-bounds write")
		}
	}

	pending = bufferdata.Pending(buffer, pending[:0])
	if len(pending) != disjointUploads {
		t.Fatalf("disjoint writes became %d uploads, want two", len(pending))
	}

	for _, upload := range pending {
		if len(upload.Data) != stride || upload.Offset != 0 && upload.Offset != rightOffset {
			t.Fatalf("upload overwrites the GPU-owned gap: %+v", upload)
		}
	}

	if owner.calls != expectedNotifications {
		t.Fatalf("dirty notifications = %d, want one per pending batch", owner.calls)
	}

	if err := buffer.Write(0, left); err != nil {
		t.Fatal(err)
	}

	if err := buffer.Write(stride, bytes.Repeat([]byte{4}, rightOffset-stride)); err != nil {
		t.Fatal(err)
	}

	if err := buffer.Write(rightOffset, right); err != nil {
		t.Fatal(err)
	}

	pending = bufferdata.Pending(buffer, pending[:0])
	if len(pending) != 1 || pending[0].Offset != 0 || len(pending[0].Data) != capacity {
		t.Fatal("adjacent writes did not form one complete upload")
	}
}
