//nolint:mnd // The wrapping-row fallback is a preferred field width in logical pixels.
package ui

import "sort"

type flowItem struct {
	capped bool
	node   *Node
	width  float32
}
type flowLine struct {
	first, end    int
	width, height float32
}

func (v *View) measureRow(node *Node, inner float32) error {
	node.flow = node.flow[:0]

	node.flowLines = node.flowLines[:0]
	for _, child := range node.children {
		if child.hidden || child.overlay {
			continue
		}

		width := flowBasis(child, node.style.Wrap, inner)

		node.flow = append(node.flow, flowItem{node: child, width: width})
	}

	line := flowLine{}

	for index, item := range node.flow {
		gap := float32(0)
		if index > line.first {
			gap = v.gap(node)
		}

		if node.style.Wrap && index > line.first && line.width+gap+item.width > inner {
			line.end = index
			node.flowLines = append(node.flowLines, line)
			line = flowLine{first: index}
			gap = 0
		}

		line.width += gap + item.width
	}

	if len(node.flow) > 0 {
		line.end = len(node.flow)
		node.flowLines = append(node.flowLines, line)
	}

	total := float32(0)

	for index := range node.flowLines {
		line := &node.flowLines[index]
		if err := v.measureFlowLine(node, line, inner); err != nil {
			return err
		}

		total += line.height
	}

	total += float32(max(0, len(node.flowLines)-1)) * v.gap(node)
	node.contentHeight = total
	node.naturalH = total + 2*node.style.Padding

	return nil
}

func (v *View) measureFlowLine(node *Node, line *flowLine, inner float32) error {
	v.distributeFlow(node, line, inner)

	for index := line.first; index < line.end; index++ {
		item := &node.flow[index]

		if err := v.measure(item.node, max(1, item.width)); err != nil {
			return err
		}

		item.width = item.node.naturalW
		line.height = max(line.height, item.node.naturalH)
	}

	return nil
}

func flowWeight(node *Node, wrap bool) float32 {
	if node.style.Width > 0 {
		return 0
	}

	if node.style.Grow > 0 {
		return node.style.Grow
	}

	if !wrap && node.style.Width == 0 && node.style.Basis == 0 {
		return 1
	}

	return 0
}

func (v *View) placeRow(node *Node, inner, clip Rect) error {
	positionY := inner.Y - node.offset
	for _, line := range node.flowLines {
		positionX := inner.X

		for _, item := range node.flow[line.first:line.end] {
			height := item.node.naturalH
			if item.node.kind == Panel && item.node.style.Height == 0 {
				height = line.height
			}

			if err := v.place(item.node, Rect{X: positionX, Y: positionY, W: item.width, H: height}, clip); err != nil {
				return err
			}

			positionX += item.width + v.gap(node)
		}

		positionY += line.height + v.gap(node)
	}

	return nil
}

type flowCap struct {
	index                   int
	headroom, weight, ratio float32
}

func (v *View) distributeFlow(node *Node, line *flowLine, inner float32) {
	remaining := max(0, inner-line.width)
	weight := float32(0)

	node.flowCaps = node.flowCaps[:0]
	for index := line.first; index < line.end; index++ {
		item := &node.flow[index]
		share := flowWeight(item.node, node.style.Wrap)

		weight += share
		if share > 0 && item.node.style.MaxWidth > 0 {
			headroom := max(0, item.node.style.MaxWidth-item.width)
			node.flowCaps = append(node.flowCaps, flowCap{index, headroom, share, headroom / share})
		}
	}

	sort.Slice(node.flowCaps, func(first, second int) bool { return node.flowCaps[first].ratio < node.flowCaps[second].ratio })

	for _, limit := range node.flowCaps {
		if weight <= 0 || remaining/weight <= limit.ratio {
			break
		}

		node.flow[limit.index].width += limit.headroom
		node.flow[limit.index].capped = true
		remaining -= limit.headroom
		weight -= limit.weight
	}

	if weight <= 0 {
		return
	}

	for index := line.first; index < line.end; index++ {
		item := &node.flow[index]
		if !item.capped {
			item.width += remaining * flowWeight(item.node, node.style.Wrap) / weight
		}
	}
}

func flowBasis(child *Node, wrap bool, inner float32) float32 {
	width := child.style.Width
	if width == 0 {
		width = child.style.Basis
	}

	if width == 0 {
		width = child.style.MinWidth
	}

	if wrap && width == 0 {
		width = 240
	}

	if wrap {
		width = min(width, inner)
	}

	width = max(child.style.MinWidth, width)
	if child.style.MaxWidth > 0 {
		width = min(width, child.style.MaxWidth)
	}

	return width
}
