package uipaint_test

import (
	"bytes"
	"testing"

	"github.com/karty-game/karty-webgpu/internal/uipaint"
)

func TestSubmissionPreservesPainterOrderAcrossSparseBlocks(t *testing.T) {
	t.Parallel()

	font, image := &uipaint.Texture{}, &uipaint.Texture{}
	source := sparseFrame(4, font)
	source.Batches[1].Texture = image
	source.Batches = append(source.Batches, uipaint.Batch{})

	var submission uipaint.Submission

	frame := submission.Update(source)
	if len(frame.Data) != 20*uipaint.Stride || len(frame.Batches) != 3 {
		t.Fatal("not densely packed in three texture runs", len(frame.Data), frame.Batches)
	}

	for index, batch := range source.Batches[:4] {
		want := source.Data[batch.First*uipaint.Stride : (batch.First+batch.Count)*uipaint.Stride]

		start := index * 5 * uipaint.Stride
		if !bytes.Equal(frame.Data[start:start+len(want)], want) {
			t.Fatal("painter record order changed", index)
		}
	}

	if frame.Batches[2].First != 10 || frame.Batches[2].Count != 10 {
		t.Fatal("adjacent same-page node runs did not merge", frame.Batches)
	}

	if len(frame.Changes) != 1 || frame.Changes[0].Size != len(frame.Data) {
		t.Fatal("initial upload should be contiguous", frame.Changes)
	}
}

//nolint:paralleltest // AllocsPerRun must run outside parallel tests.
func TestSubmissionChangesSkippedRevisionAndIdle(t *testing.T) {
	source := sparseFrame(4, &uipaint.Texture{})

	var submission uipaint.Submission

	frame := submission.Update(source)
	first := append([]byte(nil), frame.Data...)
	source.Revision, source.Base = 5, 4 // renderer skipped revisions 2–4
	source.Data[source.Batches[2].First*uipaint.Stride]++

	frame = submission.Update(source)
	if frame.Base != 1 || frame.Revision != 5 || len(frame.Changes) != 1 {
		t.Fatal("changes must refer to the last packed revision", frame.Base, frame.Revision, frame.Changes)
	}

	change := frame.Changes[0]
	if change.Offset != 10*uipaint.Stride || change.Size != 5*uipaint.Stride {
		t.Fatal("small edit uploaded unrelated blocks", change)
	}

	copy(first[change.Offset:change.Offset+change.Size], frame.Data[change.Offset:change.Offset+change.Size])

	if !bytes.Equal(first, frame.Data) {
		t.Fatal("incremental upload does not reproduce packed records")
	}

	if allocations := testing.AllocsPerRun(100, func() { submission.Update(source) }); allocations != 0 {
		t.Fatal("idle submission allocated", allocations)
	}

	source.Revision++

	frame = submission.Update(source)
	if len(frame.Changes) != 0 {
		t.Fatal("unchanged records uploaded", frame.Changes)
	}
}

func TestSubmissionReorderShrinkAndEmpty(t *testing.T) {
	t.Parallel()

	source := sparseFrame(4, &uipaint.Texture{})

	var submission uipaint.Submission
	submission.Update(source)
	source.Batches[0], source.Batches[2] = source.Batches[2], source.Batches[0]
	source.Batches = source.Batches[:2]
	source.Revision++

	frame := submission.Update(source)
	if len(frame.Data) != 10*uipaint.Stride ||
		!bytes.Equal(frame.Data[:5*uipaint.Stride], source.Data[16*uipaint.Stride:21*uipaint.Stride]) {
		t.Fatal("reordered or removed blocks retained old records")
	}

	source.Revision++
	source.Batches = nil

	frame = submission.Update(source)
	if len(frame.Data) != 0 || len(frame.Batches) != 0 || len(frame.Changes) != 0 {
		t.Fatal("empty submission retained draws/uploads", frame)
	}

	source.Revision++
	source.Batches = []uipaint.Batch{{First: 8, Count: 5, Texture: &uipaint.Texture{}}}

	frame = submission.Update(source)
	if len(frame.Data) != 5*uipaint.Stride || len(frame.Changes) != 1 || frame.Changes[0].Size != len(frame.Data) {
		t.Fatal("restoring records after empty needs upload", frame.Changes)
	}
}

//nolint:paralleltest // AllocsPerRun must run outside parallel tests.
func TestSubmissionWarmChangedFrameDoesNotAllocate(t *testing.T) {
	source := sparseFrame(100, &uipaint.Texture{})

	var submission uipaint.Submission
	submission.Update(source)

	allocations := testing.AllocsPerRun(100, func() {
		source.Revision++
		source.Data[0]++
		submission.Update(source)
	})
	if allocations != 0 {
		t.Fatal("warm changed submission allocated", allocations)
	}
}

func BenchmarkSubmission(b *testing.B) {
	for _, scenario := range []string{"idle", "one-node", "scroll"} {
		b.Run(scenario, func(b *testing.B) {
			source := sparseFrame(100, &uipaint.Texture{})

			var submission uipaint.Submission
			submission.Update(source)
			b.ReportAllocs()
			b.ResetTimer()

			for range b.N {
				if scenario != "idle" {
					source.Revision++
					source.Data[0]++
				}

				if scenario == "scroll" {
					for _, batch := range source.Batches {
						source.Data[batch.First*uipaint.Stride]++
					}
				}

				submission.Update(source)
			}
		})
	}
}

func sparseFrame(nodes int, texture *uipaint.Texture) *uipaint.Frame {
	frame := &uipaint.Frame{Revision: 1, Data: make([]byte, nodes*8*uipaint.Stride)}
	for index := range nodes {
		frame.Batches = append(frame.Batches, uipaint.Batch{First: index * 8, Count: 5, Texture: texture})
		for offset := range 5 * uipaint.Stride {
			frame.Data[index*8*uipaint.Stride+offset] = byte(index + 1)
		}
	}

	return frame
}
