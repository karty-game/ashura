package ui

const maxTextLayouts = 1024

// Entries use indices rather than heap-allocated links. Eviction removes only
// the least recently used layout, retaining hot labels during long list scrolls.
type textCacheEntry struct {
	key            textKey
	layout         textLayout
	previous, next int
}

func (font *Font) cachedLayout(key textKey) (textLayout, bool) {
	index, ok := font.cache[key]
	if !ok {
		return textLayout{}, false
	}

	font.touchLayout(index)

	return font.cacheEntries[index].layout, true
}

func (font *Font) cacheLayout(key textKey, layout textLayout) {
	index := len(font.cacheEntries)
	if len(font.cache) == maxTextLayouts {
		index = font.cacheTail
		entry := &font.cacheEntries[index]
		delete(font.cache, entry.key)
		font.unlinkLayout(index)
	} else {
		font.cacheEntries = append(font.cacheEntries, textCacheEntry{})
	}

	font.cacheEntries[index] = textCacheEntry{key: key, layout: layout}
	font.cache[key] = index
	font.touchLayout(index)
}

func (font *Font) touchLayout(index int) {
	if font.cacheHead == index {
		return
	}

	font.unlinkLayout(index)
	entry := &font.cacheEntries[index]

	entry.next = font.cacheHead
	if font.cacheHead != 0 {
		font.cacheEntries[font.cacheHead].previous = index
	}

	font.cacheHead = index
	if font.cacheTail == 0 {
		font.cacheTail = index
	}
}

func (font *Font) unlinkLayout(index int) {
	entry := &font.cacheEntries[index]
	if entry.previous != 0 {
		font.cacheEntries[entry.previous].next = entry.next
	} else if font.cacheHead == index {
		font.cacheHead = entry.next
	}

	if entry.next != 0 {
		font.cacheEntries[entry.next].previous = entry.previous
	} else if font.cacheTail == index {
		font.cacheTail = entry.previous
	}

	entry.previous, entry.next = 0, 0
}
