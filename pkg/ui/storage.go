package ui

import "github.com/karty-game/karty-webgpu/internal/uipaint"

type recordBlock struct{ slot, capacity int }

func preparedImageBytes(node *Node) int {
	pages := make(map[*uipaint.Texture]bool)
	collectImagePages(node, pages)

	bytes := 0
	for page := range pages {
		bytes += len(page.Pixels)
	}

	return bytes
}

func collectImagePages(node *Node, pages map[*uipaint.Texture]bool) {
	if page := nodeImage(node); page != nil {
		pages[page] = true
	}

	for _, child := range node.children {
		collectImagePages(child, pages)
	}
}

func (v *View) additionalImageBytes(children []*Node) int {
	pages := make(map[*uipaint.Texture]bool)
	for _, child := range children {
		collectImagePages(child, pages)
	}

	bytes := 0

	for page := range pages {
		if v.images[page] == 0 {
			bytes += len(page.Pixels)
		}
	}

	return bytes
}

func nodeImage(node *Node) *uipaint.Texture {
	if node.image != nil {
		return node.image
	}

	if page := buttonImage(node.buttonVisual); page != nil {
		return page
	}

	return panelBackgroundImage(node.panelBackground)
}

func buttonImage(visual ButtonVisual) *uipaint.Texture {
	if visual.Image != nil {
		return visual.Image.page
	}

	return nil
}

func (v *View) canReplaceImage(oldPage, newPage *uipaint.Texture) bool {
	bytes := v.imageBytes + len(v.font.page.Pixels)
	if oldPage != nil && v.images[oldPage] == 1 {
		bytes -= len(oldPage.Pixels)
	}

	if newPage != nil && (v.images[newPage] == 0 || oldPage == newPage && v.images[newPage] == 1) {
		bytes += len(newPage.Pixels)
	}

	return bytes <= MaxImageBytes
}

func (v *View) retainImage(page *uipaint.Texture) {
	if page == nil {
		return
	}

	if v.images == nil {
		v.images = make(map[*uipaint.Texture]int)
	}

	if v.images[page] == 0 {
		v.imageBytes += len(page.Pixels)
	}

	v.images[page]++
}

func (v *View) releaseImage(page *uipaint.Texture) {
	if page == nil {
		return
	}

	v.images[page]--
	if v.images[page] == 0 {
		delete(v.images, page)
		v.imageBytes -= len(page.Pixels)
	}
}

func (v *View) allocateRecords(count int) (recordBlock, error) {
	capacity := 8
	for capacity < count {
		capacity *= 2
	}

	for index, block := range v.free {
		if block.capacity >= capacity {
			v.free[index] = v.free[len(v.free)-1]
			v.free = v.free[:len(v.free)-1]

			return block, nil
		}
	}

	if v.used+capacity > MaxRecords {
		return recordBlock{}, ErrBounds
	}

	block := recordBlock{v.used, capacity}
	v.used += capacity

	return block, nil
}
