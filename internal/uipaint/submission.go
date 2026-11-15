package uipaint

import "bytes"

// Submission retains a dense GPU submission independently of the View's stable
// record slots. Only a changed revision is copied/compared. Adjacent painter
// spans sharing a page become a single draw, even across sparse node blocks.
// This is draw-record bookkeeping; immutable asset pixels are never transformed.
type Submission struct {
	frame Frame
	ready bool
}

// Current returns the last prepared submission, borrowed until the next Update.
func (submission *Submission) Current() *Frame { return &submission.frame }

// Update returns retained packed records and only their changed byte ranges.
// Source revisions may be skipped: comparison is against our own last packed
// submission, so Base always names the buffer whose contents Changes update.
func (submission *Submission) Update(source *Frame) *Frame {
	frame := &submission.frame

	frame.Width, frame.Height = source.Width, source.Height
	if submission.ready && frame.Revision == source.Revision {
		return frame
	}

	frame.Base, frame.Revision = frame.Revision, source.Revision
	submission.ready = true
	frame.Changes = frame.Changes[:0]

	count := 0
	for _, batch := range source.Batches {
		count += batch.Count
	}

	previous := len(frame.Data)

	size := count * Stride
	if size > cap(frame.Data) {
		data := make([]byte, size, max(size, cap(frame.Data)*2))
		copy(data, frame.Data)
		frame.Data = data
	} else {
		frame.Data = frame.Data[:size]
	}

	previousBatches := len(frame.Batches)
	frame.Batches = frame.Batches[:0]
	first := 0

	for _, batch := range source.Batches {
		if batch.Count == 0 {
			continue
		}

		start, end := first*Stride, (first+batch.Count)*Stride

		records := source.Data[batch.First*Stride : (batch.First+batch.Count)*Stride]
		if end > previous || !bytes.Equal(frame.Data[start:end], records) {
			copy(frame.Data[start:end], records)
			frame.Changes = appendChange(frame.Changes, start, end-start)
		}

		last := len(frame.Batches) - 1
		if last >= 0 && frame.Batches[last].Texture == batch.Texture {
			frame.Batches[last].Count += batch.Count
		} else {
			frame.Batches = append(frame.Batches, Batch{First: first, Count: batch.Count, Texture: batch.Texture})
		}

		first += batch.Count
	}

	if len(frame.Batches) < previousBatches {
		clear(frame.Batches[len(frame.Batches):previousBatches])
	}

	return frame
}

func appendChange(changes []Range, offset, size int) []Range {
	last := len(changes) - 1
	if last >= 0 && changes[last].Offset+changes[last].Size == offset {
		changes[last].Size += size

		return changes
	}

	return append(changes, Range{Offset: offset, Size: size})
}
