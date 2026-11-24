package level

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/karty-game/ashura/internal/resourcedata"
	"github.com/karty-game/ashura/pkg/scene/resource"
)

type referenceResult struct {
	payloads   []*resource.Payload
	err        error
	unfinished int
}

var errReferenceIntegrityFixture = errors.New("original prepared payload integrity failure")

func scheduledReferences(count, size int) []Reference {
	refs := make([]Reference, count)
	for index := range refs {
		refs[index] = Reference{Path: fmt.Sprintf("%d.bin", index), Size: size}
	}

	return refs
}

func scheduledPayload(ref Reference) *resource.Payload {
	data := []byte(ref.Path)

	return resourcedata.OwnVerified(data, sha256.Sum256(data))
}

func awaitReferenceValue[T any](t *testing.T, values <-chan T) T {
	t.Helper()

	select {
	case value := <-values:
		return value
	case <-time.After(5 * time.Second):
		t.Fatal("reference scheduler did not reach its synchronization barrier")
	}

	var zero T

	return zero
}

func TestReferenceReadsOverlapWithinWorkerBound(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	refs := scheduledReferences(3*concurrentReferenceReads, 1)
	started := make(chan struct{}, len(refs))
	gate := make(chan struct{})

	release := sync.OnceFunc(func() { close(gate) })
	defer release()

	done := make(chan referenceResult, 1)

	var guard sync.Mutex

	active, maximum, calls := 0, 0, 0

	go func() {
		values, err := readReferences(ctx, refs, func(ctx context.Context, ref Reference) (*resource.Payload, error) {
			guard.Lock()
			active++
			calls++
			maximum = max(maximum, active)

			guard.Unlock()
			defer func() { guard.Lock(); active--; guard.Unlock() }()

			started <- struct{}{}

			select {
			case <-gate:
				return scheduledPayload(ref), nil
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		})
		done <- referenceResult{payloads: values, err: err}
	}()
	// All eight readers must start while none can finish. This fails a serial loop
	// through synchronization, without using elapsed time as a throughput assertion.
	for range concurrentReferenceReads {
		awaitReferenceValue(t, started)
	}

	release()

	result := awaitReferenceValue(t, done)
	if result.err != nil || len(result.payloads) != len(refs) {
		t.Fatal("parallel reads failed", result.err, len(result.payloads))
	}

	for index, payload := range result.payloads {
		if string(resourcedata.Bytes(payload)) != refs[index].Path {
			t.Fatal("completion order changed reference association", index)
		}
	}

	guard.Lock()
	defer guard.Unlock()

	if maximum != concurrentReferenceReads || active != 0 || calls != len(refs) {
		t.Fatal("worker limit, join or one-read-per-reference contract failed", maximum, active, calls)
	}
}

func TestReferenceReadsBoundBytesAndRunOversizedFileAlone(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	// Declared sizes exercise admission without allocating a 192 MiB fixture. The
	// injected reader supplies small immutable payloads; real integrity is tested
	// independently through ReadChunk and the production resource reader.
	refs := scheduledReferences(9, 8<<20)
	refs[4].Size = 128 << 20
	gates := [3]chan struct{}{make(chan struct{}), make(chan struct{}), make(chan struct{})}

	releases := [3]func(){}
	for index := range gates {
		releases[index] = sync.OnceFunc(func() { close(gates[index]) })
		defer releases[index]()
	}

	started := make(chan Reference, len(refs))
	done := make(chan referenceResult, 1)

	var guard sync.Mutex

	active, bytes, violations := 0, 0, 0

	go func() {
		values, err := readReferences(ctx, refs, func(ctx context.Context, ref Reference) (*resource.Payload, error) {
			group := 2

			switch ref.Path {
			case "0.bin", "1.bin", "2.bin", "3.bin":
				group = 0
			case "4.bin":
				group = 1
			}

			guard.Lock()
			active++

			bytes += ref.Size
			if active > concurrentReferenceReads || bytes > activeReferenceBytes && active != 1 {
				violations++
			}

			guard.Unlock()
			defer func() { guard.Lock(); active--; bytes -= ref.Size; guard.Unlock() }()

			started <- ref

			select {
			case <-gates[group]:
				return scheduledPayload(ref), nil
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		})
		done <- referenceResult{payloads: values, err: err}
	}()

	for range 4 {
		if ref := awaitReferenceValue(t, started); ref.Size != 8<<20 {
			t.Fatal("oversized file overlapped earlier reads", ref)
		}
	}

	releases[0]()

	if ref := awaitReferenceValue(t, started); ref.Path != "4.bin" {
		t.Fatal("oversized file lost queue ownership", ref)
	}

	guard.Lock()
	alone := active == 1 && bytes == 128<<20
	guard.Unlock()

	if !alone {
		t.Fatal("oversized file was not read alone")
	}

	releases[1]()

	for range 4 {
		awaitReferenceValue(t, started)
	}

	releases[2]()

	result := awaitReferenceValue(t, done)
	if result.err != nil || len(result.payloads) != len(refs) {
		t.Fatal("bounded reads failed", result.err)
	}

	guard.Lock()
	defer guard.Unlock()

	if violations != 0 || active != 0 || bytes != 0 {
		t.Fatal("in-flight byte limit or join failed", violations, active, bytes)
	}
}

func TestReferenceReadFailureCancelsAndJoinsEveryStartedReader(t *testing.T) {
	t.Parallel()

	for _, external := range []bool{false, true} {
		t.Run(fmt.Sprintf("parent-cancellation-%t", external), func(t *testing.T) {
			t.Parallel()

			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()

			refs := scheduledReferences(2*concurrentReferenceReads, 1)
			started := make(chan Reference, len(refs))
			canceled := make(chan struct{}, len(refs))
			fail := make(chan struct{})

			trigger := sync.OnceFunc(func() { close(fail) })
			defer trigger()

			allowJoin := make(chan struct{})

			release := sync.OnceFunc(func() { close(allowJoin) })
			defer release()

			done := make(chan referenceResult, 1)
			failure := errReferenceIntegrityFixture

			var guard sync.Mutex

			calls, unfinished := 0, 0

			go func() {
				values, err := readReferences(ctx, refs, func(ctx context.Context, ref Reference) (*resource.Payload, error) {
					guard.Lock()
					calls++
					unfinished++

					guard.Unlock()
					defer func() { guard.Lock(); unfinished--; guard.Unlock() }()

					started <- ref

					if ref.Path == "0.bin" && !external {
						<-fail

						return nil, failure
					}

					<-ctx.Done()

					canceled <- struct{}{}

					<-allowJoin

					return nil, ctx.Err()
				})

				guard.Lock()
				remaining := unfinished
				guard.Unlock()

				done <- referenceResult{payloads: values, err: err, unfinished: remaining}
			}()

			for range concurrentReferenceReads {
				awaitReferenceValue(t, started)
			}

			expectedCanceled := concurrentReferenceReads - 1
			expected := failure

			if external {
				cancel()

				expectedCanceled = concurrentReferenceReads
				expected = context.Canceled
			} else {
				trigger()
			}

			for range expectedCanceled {
				awaitReferenceValue(t, canceled)
			}
			// The readers have observed cancellation but cannot return until explicitly
			// released. The scheduler must keep ownership while those readers are alive.
			select {
			case result := <-done:
				t.Fatal("scheduler returned before canceled readers completed", result.err)
			default:
			}

			release()

			result := awaitReferenceValue(t, done)
			if !errors.Is(result.err, expected) || result.payloads != nil || result.unfinished != 0 {
				t.Fatal("cancellation lost cause, partial result or worker ownership", result.err, result.unfinished)
			}

			guard.Lock()
			defer guard.Unlock()

			if calls != concurrentReferenceReads || unfinished != 0 {
				t.Fatal("cancellation started queued work or left readers alive", calls, unfinished)
			}
		})
	}
}

func TestCanceledReferenceReadsStartNoReader(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	called := false

	payloads, err := readReferences(
		ctx,
		scheduledReferences(1, 1),
		func(context.Context, Reference) (*resource.Payload, error) {
			called = true

			return scheduledPayload(Reference{Path: "unexpected"}), nil
		},
	)
	if !errors.Is(err, context.Canceled) || payloads != nil || called {
		t.Fatal("canceled request started work", payloads, err, called)
	}
}
