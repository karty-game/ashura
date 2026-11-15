//nolint:mnd // Analytic primitive modes and antialias bounds are the shared GPU ABI.
package ui

import "math"

type chartPoint struct{ x, y float32 }

func (v *View) chartStroke(node *Node, first, last chartPoint, clip Rect, color Color, width float32) {
	if width <= 0 || !finiteChartPoints(first, last) {
		return
	}

	pad := width/2 + 1
	left, top := min(first.x, last.x)-pad, min(first.y, last.y)-pad
	right, bottom := max(first.x, last.x)+pad, max(first.y, last.y)+pad
	node.paint = append(node.paint, paintRecord{
		rect: Rect{X: left, Y: top, W: right - left, H: bottom - top}, clip: clip, color: color,
		uv: Rect{X: first.x, Y: first.y, W: last.x, H: last.y}, radius: width / 2, mode: 3,
	})
}

func (v *View) chartTriangle(node *Node, first, second, third chartPoint, clip Rect, color Color) {
	if !finiteChartPoints(first, second, third) {
		return
	}

	area := (second.x-first.x)*(third.y-first.y) - (second.y-first.y)*(third.x-first.x)
	if abs(area) < 0.00001 {
		return
	}

	left, top := min(first.x, second.x, third.x)-1, min(first.y, second.y, third.y)-1
	right, bottom := max(first.x, second.x, third.x)+1, max(first.y, second.y, third.y)+1
	node.paint = append(node.paint, paintRecord{
		rect: Rect{X: left, Y: top, W: right - left, H: bottom - top}, clip: clip, color: color,
		uv: Rect{X: first.x, Y: first.y, W: second.x, H: second.y}, radius: third.x, borderWidth: third.y, mode: 4,
	})
}

func (v *View) chartSector(node *Node, center chartPoint, radius, start, end float32, clip Rect, color Color) {
	if !finiteChartPoints(center) || !finite(radius) || !finite(start) || !finite(end) || radius <= 0 || end <= start {
		return
	}

	first, last := chartDirection(start), chartDirection(end)
	left, right := min(
		center.x,
		center.x+radius*first.x,
		center.x+radius*last.x,
	), max(
		center.x,
		center.x+radius*first.x,
		center.x+radius*last.x,
	)
	top, bottom := min(
		center.y,
		center.y+radius*first.y,
		center.y+radius*last.y,
	), max(
		center.y,
		center.y+radius*first.y,
		center.y+radius*last.y,
	)

	for index := range 5 {
		angle := float32(index) * math.Pi / 2
		if angle >= start && angle <= end {
			point := chartDirection(angle)
			left, right = min(left, center.x+radius*point.x), max(right, center.x+radius*point.x)
			top, bottom = min(top, center.y+radius*point.y), max(bottom, center.y+radius*point.y)
		}
	}

	// Sector center lives in the otherwise unused border payload; UV stores
	// unit boundary rays, and params store radius/sweep. Bounds only cover the wedge.
	node.paint = append(node.paint, paintRecord{
		rect: Rect{X: left - 1, Y: top - 1, W: right - left + 2, H: bottom - top + 2}, clip: clip, color: color,
		border: Color{center.x, center.y, 0, 0}, uv: Rect{X: first.x, Y: first.y, W: last.x, H: last.y},
		radius: radius, borderWidth: min(end-start, 2*math.Pi), mode: 5,
	})
}

func chartDirection(angle float32) chartPoint {
	return chartPoint{float32(math.Sin(float64(angle))), -float32(math.Cos(float64(angle)))}
}

func finiteChartPoints(points ...chartPoint) bool {
	for _, point := range points {
		if !finite(point.x) || !finite(point.y) {
			return false
		}
	}

	return true
}
