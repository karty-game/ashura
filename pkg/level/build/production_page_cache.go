//go:build !js

package levelbuild

import (
	"context"
	"strings"
	"sync"

	"github.com/karty-game/ashura/pkg/assetbuild"
	bake "github.com/karty-game/ashura/pkg/lightmap/bake"
)

type productionPageKey struct {
	geometry         string
	receivers, model assetbuild.Artifact
}

type productionPageEntry struct {
	ready chan struct{}
	set   bake.ReceiverSet
	err   error
}

type productionPageRawKey struct{ receivers, model assetbuild.Artifact }

// All consumers declare both artifacts on their actions. The runner verifies
// those complete inputs before Run, including on a memo hit. Only the admitted,
// remapped typed records are shared; callers borrow their immutable slices.
// Resolve releases its logical space and build cleanup releases failed work.
type productionPageCache struct {
	mu     sync.Mutex
	pages  map[productionPageKey]*productionPageEntry
	groups map[string][]bake.Receiver
	raw    map[productionPageRawKey]bake.ReceiverSet
}

// Integrated UV production transfers its already validated, owned footprints.
// Material remapping consumes this ownership once; another geometry identity
// falls back to the admitted artifact decoder rather than sharing mutable input.
func (cache *productionPageCache) seed(page pageRecord, set bake.ReceiverSet) {
	if cache == nil {
		return
	}

	cache.mu.Lock()
	defer cache.mu.Unlock()

	if cache.raw == nil {
		cache.raw = make(map[productionPageRawKey]bake.ReceiverSet)
	}

	cache.raw[productionPageRawKey{page.Receivers, page.Geometry}] = set
}

func (cache *productionPageCache) take(page pageRecord) (bake.ReceiverSet, bool) {
	if cache == nil {
		return bake.ReceiverSet{}, false
	}

	cache.mu.Lock()
	defer cache.mu.Unlock()

	key := productionPageRawKey{page.Receivers, page.Geometry}
	set, found := cache.raw[key]
	delete(cache.raw, key)

	return set, found
}

func (cache *productionPageCache) get(
	ctx context.Context,
	key productionPageKey,
	load func() (bake.ReceiverSet, error),
) (bake.ReceiverSet, error) {
	if cache == nil {
		return load()
	}

	cache.mu.Lock()

	entry := cache.pages[key]
	if entry == nil {
		entry = &productionPageEntry{ready: make(chan struct{})}
		if cache.pages == nil {
			cache.pages = make(map[productionPageKey]*productionPageEntry)
		}

		cache.pages[key] = entry
		cache.mu.Unlock()

		entry.set, entry.err = load()
		close(entry.ready)
	} else {
		cache.mu.Unlock()

		select {
		case <-entry.ready:
		case <-ctx.Done():
			return bake.ReceiverSet{}, ctx.Err()
		}
	}

	return entry.set, entry.err
}

func productionReceiverGroupKey(scene *bake.PreparedScene, pages []pageRecord) string {
	var key strings.Builder
	key.WriteString(scene.GeometryHash())

	for _, page := range pages {
		// SHA256 strings have an admitted fixed length. Including both ordered
		// references prevents page-roster or material-remap aliases.
		key.WriteString(page.Receivers.SHA256)
		key.WriteString(page.Geometry.SHA256)
	}

	return key.String()
}

func (cache *productionPageCache) group(key string) []bake.Receiver {
	if cache == nil {
		return nil
	}

	cache.mu.Lock()
	defer cache.mu.Unlock()

	return cache.groups[key]
}

func (cache *productionPageCache) rememberGroup(key string, receivers []bake.Receiver) {
	if cache == nil {
		return
	}

	cache.mu.Lock()
	defer cache.mu.Unlock()

	if cache.groups == nil {
		cache.groups = make(map[string][]bake.Receiver)
	}

	cache.groups[key] = receivers
}

func (plugin *productionPlugin) releaseProductionPages(scene *bake.PreparedScene, pages []pageRecord) {
	cache := plugin.pageInputs
	if cache == nil {
		return
	}

	cache.mu.Lock()
	defer cache.mu.Unlock()

	delete(cache.groups, productionReceiverGroupKey(scene, pages))

	for _, page := range pages {
		delete(cache.pages, productionPageKey{scene.GeometryHash(), page.Receivers, page.Geometry})
		delete(cache.raw, productionPageRawKey{page.Receivers, page.Geometry})
	}
}

func (plugin *productionPlugin) readProductionReceiverPage(
	ctx context.Context,
	scene *bake.PreparedScene,
	page pageRecord,
	read func(assetbuild.Artifact) ([]byte, error),
) (bake.ReceiverSet, error) {
	return plugin.pageInputs.get(
		ctx,
		productionPageKey{scene.GeometryHash(), page.Receivers, page.Geometry},
		func() (bake.ReceiverSet, error) {
			set, found := plugin.pageInputs.take(page)
			if !found {
				data, err := read(page.Receivers)
				if err != nil {
					return bake.ReceiverSet{}, err
				}

				set, err = bake.DecodeReceiverSet(data)
				if err != nil {
					return bake.ReceiverSet{}, err
				}
			}

			geometry, err := read(page.Geometry)
			if err != nil {
				return bake.ReceiverSet{}, err
			}

			if err = bake.RemapReceiverMaterials(geometry, scene, set.Receivers); err != nil {
				return bake.ReceiverSet{}, err
			}

			return set, nil
		},
	)
}
