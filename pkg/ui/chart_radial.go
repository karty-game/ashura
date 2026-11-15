//nolint:mnd // Radial chart label gutters, grid levels and fill opacity are presentation metrics.
package ui

import "math"

func (v *View) paintPieChart(node *Node, box Rect) {
	radius := max(0, min(box.W, box.H)/2-6)
	if radius <= 0 {
		return
	}

	center := chartPoint{box.X + box.W/2, box.Y + box.H/2}
	values := node.chart.data.Series[0].Values

	maximum := float64(0)
	for _, value := range values {
		maximum = max(maximum, value)
	}

	total := chartPieTotal(values)
	angle := float32(0)

	for index, value := range values {
		if value == 0 {
			continue
		}

		next := angle + float32(value/maximum/total*2*math.Pi)
		v.chartSector(node, center, radius, angle, next, node.clip.Intersect(box), v.chartColor(node, index))
		angle = next
	}
}

func (v *View) paintRadarChart(node *Node, box Rect) error {
	labelSize := v.chartFontSize(node)

	radius := max(0, min(box.W/2-46, box.H/2-labelSize*1.7))
	if radius < 8 {
		return nil
	}

	center := chartPoint{box.X + box.W/2, box.Y + box.H/2}
	labels := node.chart.data.Labels

	clip := node.clip.Intersect(box)
	if !node.chart.options.HideGrid {
		v.chartRadarGrid(node, center, radius, clip)
	}

	for index, label := range labels {
		point := chartPolar(center, radius+12, index, len(labels))

		bounds := Rect{X: point.x - 42, Y: point.y - labelSize*0.75, W: 84, H: labelSize * 1.5}
		if err := v.chartLabel(node, label, bounds, 0.5, v.theme.Muted); err != nil {
			return err
		}
	}

	domain := chartDomain(node.chart)
	for seriesIndex, series := range node.chart.data.Series {
		color := v.chartColor(node, seriesIndex)
		fill := color
		fill[3] *= 0.16

		for index, value := range series.Values {
			point := chartPolar(center, radius*chartFraction(value, domain), index, len(labels))
			nextIndex := (index + 1) % len(labels)
			next := chartPolar(center, radius*chartFraction(series.Values[nextIndex], domain), nextIndex, len(labels))
			v.chartTriangle(node, center, point, next, clip, fill)
			v.chartStroke(node, point, next, clip, color, chartLineWidth(node))
			v.box(node, Rect{X: point.x - 2, Y: point.y - 2, W: 4, H: 4}, clip, color, Color{}, 2, 0)
		}
	}

	return nil
}

func (v *View) chartRadarGrid(node *Node, center chartPoint, radius float32, clip Rect) {
	count := len(node.chart.data.Labels)
	color := v.chartGridColor(node)

	for level := 1; level <= 4; level++ {
		for index := range count {
			point := chartPolar(center, radius*float32(level)/4, index, count)
			next := chartPolar(center, radius*float32(level)/4, (index+1)%count, count)
			v.chartStroke(node, point, next, clip, color, 1)
		}
	}

	for index := range count {
		v.chartStroke(node, center, chartPolar(center, radius, index, count), clip, color, 1)
	}
}

func chartPolar(center chartPoint, radius float32, index, count int) chartPoint {
	angle := float64(index)/float64(count)*2*math.Pi - math.Pi/2

	return chartPoint{center.x + radius*float32(math.Cos(angle)), center.y + radius*float32(math.Sin(angle))}
}
