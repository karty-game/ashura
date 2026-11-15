package ui

import (
	"strconv"
	"testing"
)

//nolint:paralleltest // Allocation counters require serial execution.
func TestTextCacheRetainsHotLayoutsWhileEvictingOneColdEntry(t *testing.T) {
	font := &Font{cache: make(map[textKey]int), cacheEntries: make([]textCacheEntry, 1, maxTextLayouts+1)}
	hot := textKey{text: "persistent heading"}
	cold := textKey{text: "old row"}
	font.cacheLayout(hot, textLayout{text: hot.text})
	font.cacheLayout(cold, textLayout{text: cold.text})

	for index := range maxTextLayouts * 3 {
		if layout, ok := font.cachedLayout(hot); !ok || layout.text != hot.text {
			t.Fatal("scrolling unique rows evicted a frequently used heading", index)
		}

		key := textKey{text: strconv.Itoa(index)}
		font.cacheLayout(key, textLayout{text: key.text})
	}

	if len(font.cache) != maxTextLayouts || len(font.cacheEntries) != maxTextLayouts+1 {
		t.Fatal("cache grew beyond its bound", len(font.cache), len(font.cacheEntries))
	}

	if _, ok := font.cachedLayout(cold); ok {
		t.Fatal("cold layout was not evicted")
	}

	seen := make(map[int]bool)
	previous := 0

	for index := font.cacheHead; index != 0; index = font.cacheEntries[index].next {
		entry := font.cacheEntries[index]
		if seen[index] || entry.previous != previous || font.cache[entry.key] != index {
			t.Fatal("LRU links or lookup are inconsistent", index, previous, entry)
		}

		seen[index] = true
		previous = index
	}

	if previous != font.cacheTail || len(seen) != maxTextLayouts {
		t.Fatal("cache chain lost entries", len(seen), previous, font.cacheTail)
	}

	if allocations := testing.AllocsPerRun(100, func() { _, _ = font.cachedLayout(hot) }); allocations != 0 {
		t.Fatal("cache hits allocate", allocations)
	}
}
