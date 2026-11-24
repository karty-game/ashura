package level

import (
	"context"
	"fmt"
	"sync"

	"github.com/karty-game/ashura/internal/leveldata"
	"github.com/karty-game/ashura/pkg/scene/resource"
)

const (
	concurrentReferenceReads = 8
	activeReferenceBytes     = 32 << 20
)

type referenceArrival[T any] struct {
	index   int
	payload T
}

type referencePayload struct {
	file    *resource.Payload
	entries []*resource.Payload
}

func (prepared *PreparedChunk) readFiles(ctx context.Context, base string, refs []Reference) error {
	missing := make([]Reference, 0, len(refs))
	for _, ref := range refs {
		if info, found := prepared.resident[ref.SHA256]; found {
			if info.size != ref.Size {
				return leveldata.ErrFormat
			}

			continue
		}

		missing = append(missing, ref)
	}

	return prepared.readMissingFiles(ctx, base, missing)
}

func (prepared *PreparedChunk) readMissingFiles(ctx context.Context, base string, missing []Reference) error {
	targets, bundle := prepared.readTargets(missing)

	payloads, err := readReferences(ctx, targets, func(ctx context.Context, ref Reference) (referencePayload, error) {
		asset := prepared.resourceAsset(ref)
		if ref == bundle.file {
			file, slices, err := resource.ReadVerifiedSlices(ctx, base, asset, bundle.entries)

			return referencePayload{file: file, entries: slices}, err
		}

		file, err := resource.ReadVerified(ctx, base, asset)

		return referencePayload{file: file}, err
	})
	if err != nil {
		return fmt.Errorf("chunk %s: %w", prepared.chunk.ID, err)
	}

	for index, ref := range targets {
		result := payloads[index]
		if len(result.entries) == 0 || bundle.retainFile {
			prepared.payloads[ref.SHA256] = result.file
		}

		for entryIndex, data := range result.entries {
			prepared.payloads[bundle.entries[entryIndex].Asset.SHA256] = data
		}
	}

	return nil
}

// References have already passed the complete chunk admission, deduplication
// and 256 MiB aggregate bound. This window bounds concurrent browser staging
// and checksum work as well as transport: at most eight reads and 32 MiB of
// declared payloads, with a larger valid file read alone. Completed GPU-ready
// bytes retain the existing aggregate bound; no App work runs in these tasks.
func readReferences[T any](
	ctx context.Context,
	refs []Reference,
	read func(context.Context, Reference) (T, error),
) ([]T, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	owned, cancel := context.WithCancelCause(ctx)

	var workers sync.WaitGroup

	defer func() {
		cancel(nil)
		workers.Wait()
	}()

	completed := make(chan referenceArrival[T], concurrentReferenceReads)
	payloads := make([]T, len(refs))
	next, active, bytes := 0, 0, 0

	for next < len(refs) || active > 0 {
		for next < len(refs) && active < concurrentReferenceReads && owned.Err() == nil &&
			(active == 0 || refs[next].Size <= activeReferenceBytes-bytes) {
			index := next
			next++
			active++
			bytes += refs[index].Size

			workers.Go(func() {
				payload, err := read(owned, refs[index])
				if err != nil {
					// Publish the substantive failure before waking canceled siblings.
					cancel(err)
				}

				completed <- referenceArrival[T]{index: index, payload: payload}
			})
		}

		select {
		case item := <-completed:
			payloads[item.index] = item.payload
			active--
			bytes -= refs[item.index].Size
		case <-owned.Done():
			return nil, referenceReadError(ctx, owned)
		}
	}

	if owned.Err() != nil {
		return nil, referenceReadError(ctx, owned)
	}

	return payloads, nil
}

func referenceReadError(parent, owned context.Context) error {
	if err := parent.Err(); err != nil {
		return err
	}

	return context.Cause(owned)
}
