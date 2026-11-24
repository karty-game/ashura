//go:build !js

package levelbuild

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"

	bake "github.com/karty-game/ashura/pkg/lightmap/bake"
)

func TestProductionPageCacheJoinsImmutablePreparation(t *testing.T) {
	t.Parallel()

	cache := &productionPageCache{}
	key := productionPageKey{geometry: "one logical geometry"}
	want := bake.ReceiverSet{Receivers: []bake.Receiver{{ID: 17}}}

	var calls atomic.Uint32

	entered, release := make(chan struct{}), make(chan struct{})

	var joined sync.WaitGroup

	for range 8 {
		joined.Go(func() {
			got, err := cache.get(t.Context(), key, func() (bake.ReceiverSet, error) {
				calls.Add(1)
				close(entered)
				<-release

				return want, nil
			})
			if err != nil || len(got.Receivers) != 1 || &got.Receivers[0] != &want.Receivers[0] {
				t.Error("consumers did not share the admitted immutable page", err)
			}
		})
	}

	<-entered
	close(release)
	joined.Wait()

	if calls.Load() != 1 {
		t.Fatal("decoded the same geometry/page more than once", calls.Load())
	}

	key.geometry = "another logical geometry"
	if _, err := cache.get(t.Context(), key, func() (bake.ReceiverSet, error) {
		calls.Add(1)

		return want, nil
	}); err != nil || calls.Load() != 2 {
		t.Fatal("geometry identity aliased a material remap", err, calls.Load())
	}

	key.model.Size++
	if _, err := cache.get(t.Context(), key, func() (bake.ReceiverSet, error) {
		calls.Add(1)

		return want, nil
	}); err != nil || calls.Load() != 3 {
		t.Fatal("artifact identity aliased a decoded page", err, calls.Load())
	}
}

func TestProductionPageCacheWaitCancellation(t *testing.T) {
	t.Parallel()

	cache := &productionPageCache{}
	key := productionPageKey{geometry: "pending page"}

	entered, release, finished := make(chan struct{}), make(chan struct{}), make(chan struct{})
	go func() {
		defer close(finished)

		_, _ = cache.get(t.Context(), key, func() (bake.ReceiverSet, error) {
			close(entered)
			<-release

			return bake.ReceiverSet{}, nil
		})
	}()

	<-entered

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	_, err := cache.get(ctx, key, func() (bake.ReceiverSet, error) {
		t.Error("waiting consumer replaced the page's producer")

		return bake.ReceiverSet{}, nil
	})

	close(release)
	<-finished

	if !errors.Is(err, context.Canceled) {
		t.Fatal("page wait ignored cancellation", err)
	}
}
