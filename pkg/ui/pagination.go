//nolint:mnd // Page bounds, fixed window sizes, button indices and spacing are explicit.
package ui

import "strconv"

const maxPageCount = 1_000_000_000

type paginationState struct {
	page, count int
	numbers     []*Node
	pages       [5]int
	first, prev *Node
	next, last  *Node
}

// NewPagination builds a bounded page window and first/previous/next/last actions.
// Page numbers start at one. No nodes are allocated for undisplayed pages.
func NewPagination(label string, page, pageCount int, onChange func(Change)) *Node {
	root := Column().WithStyle(Style{Gap: 6})
	if !validNavigationString(label) || pageCount < 1 || pageCount > maxPageCount || page < 1 || page > pageCount {
		root.constructionErr = ErrBounds

		return root
	}

	state := &paginationState{count: pageCount, numbers: make([]*Node, 0, 5)}
	root.pagination, root.role, root.label, root.change = state, "navigation", label, onChange
	state.first = NewButton("«", func() { root.activatePage(1) })
	state.prev = NewButton("‹", func() { root.activatePage(state.page - 1) })
	state.next = NewButton("›", func() { root.activatePage(state.page + 1) })
	state.last = NewButton("»", func() { root.activatePage(state.count) })
	state.first.label, state.prev.label = "First page", "Previous page"
	state.next.label, state.last.label = "Next page", "Last page"
	window := Row().WithStyle(Style{Gap: 6})

	for index := range state.pages {
		button := NewButton("", func() { root.activatePage(state.pages[index]) }).WithStyle(Style{FontSize: 13})

		state.numbers = append(state.numbers, button)
		if err := window.Add(button); err != nil {
			root.constructionErr = err

			return root
		}
	}

	root.constructionErr = root.Add(window, Row(state.first, state.prev, state.next, state.last).WithStyle(Style{Gap: 6}))
	root.applyPage(page)

	return root
}

// Page returns the current page, or zero for other nodes.
func (node *Node) Page() int {
	if node.pagination == nil {
		return 0
	}

	return node.pagination.page
}

// PageCount returns the total page count, or zero for other nodes.
func (node *Node) PageCount() int {
	if node.pagination == nil {
		return 0
	}

	return node.pagination.count
}

// SetPage selects a page silently. The requested page must exist.
func (node *Node) SetPage(page int) error {
	if node.pagination == nil || page < 1 || page > node.pagination.count {
		return ErrBounds
	}

	if node.pagination.page != page {
		node.applyPage(page)
	}

	return nil
}

// SetPageCount changes the total silently and clamps the current page if needed.
func (node *Node) SetPageCount(count int) error {
	if node.pagination == nil || count < 1 || count > maxPageCount {
		return ErrBounds
	}

	if node.pagination.count != count {
		node.pagination.count = count
		node.applyPage(min(node.pagination.page, count))
	}

	return nil
}

func (node *Node) activatePage(page int) {
	if page < 1 || page > node.pagination.count || page == node.pagination.page {
		return
	}

	view := node.view
	node.applyPage(page)

	if node.view == view {
		node.changeValue()
	}
}

func (node *Node) applyPage(page int) {
	state := node.pagination
	view := node.view
	focused := view != nil && withinNode(view.focus, node)
	state.page, node.value, node.text = page, float64(page), strconv.Itoa(page)
	node.minimum, node.maximum = 1, float64(state.count)
	state.first.SetDisabled(page == 1)
	state.prev.SetDisabled(page == 1)
	state.next.SetDisabled(page == state.count)
	state.last.SetDisabled(page == state.count)
	window := min(5, state.count)

	if state.count >= 100_000 {
		window = 3
	}

	first := max(1, min(page-window/2, state.count-window+1))
	for index, button := range state.numbers {
		number := first + index
		state.pages[index] = number
		button.SetHidden(index >= window)
		button.semanticSelected = number == page

		button.current = ""
		if number == page {
			button.current = "page"
		}

		button.SetChecked(number == page)
		button.text = strconv.Itoa(number)
		button.label = "Page " + button.text
		button.damage(true)
	}

	node.damage(true)

	if focused && node.view == view && view.focus == nil {
		view.SetFocus(state.numbers[page-first])
	}
}
