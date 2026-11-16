package level

import (
	"context"
	"fmt"
	"sync"

	"github.com/karty-game/karty-webgpu/internal/leveldata"
	"github.com/karty-game/karty-webgpu/internal/resourcedata"
	"github.com/karty-game/karty-webgpu/pkg/scene/resource"
)

const (
	concurrentReferenceReads = 8
	activeReferenceBytes     = 32 << 20
)

type referenceArrival struct {
	index   int
	payload *resource.Payload
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

	payloads, err := readReferences(ctx, missing, func(ctx context.Context, ref Reference) (*resource.Payload, error) {
		return resource.ReadVerified(ctx, base, resource.Asset{ID: prepared.chunk.ID, Path: ref.Path, Size: ref.Size, SHA256: ref.SHA256})
	})
	if err != nil {
		return fmt.Errorf("chunk %s: %w", prepared.chunk.ID, err)
	}

	for index, ref := range missing {
		data := payloads[index]
		prepared.verified[ref.SHA256] = data
		prepared.files[ref.SHA256] = resourcedata.Bytes(data)
	}

	return nil
}

// References have already passed the complete chunk admission, deduplication
// and 256 MiB aggregate bound. This window bounds concurrent browser staging
// and checksum work as well as transport: at most eight reads and 32 MiB of
// declared payloads, with a larger valid file read alone. Completed GPU-ready
// bytes retain the existing aggregate bound; no App work runs in these tasks.
func readReferences(
	ctx context.Context,
	refs []Reference,
	read func(context.Context, Reference) (*resource.Payload, error),
) ([]*resource.Payload, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	owned, cancel := context.WithCancelCause(ctx)

	var workers sync.WaitGroup

	defer func() {
		cancel(nil)
		workers.Wait()
	}()

	completed := make(chan referenceArrival, concurrentReferenceReads)
	payloads := make([]*resource.Payload, len(refs))
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

				completed <- referenceArrival{index: index, payload: payload}
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
