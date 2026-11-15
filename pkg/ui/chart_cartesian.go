//nolint:mnd // Chart plot gutters, tick counts and marker radii are presentation metrics.
package ui

import "math"

func (v *View) paintCartesianChart(node *Node, box Rect) error {
	labelHeight := v.chartFontSize(node) * 1.5

	plot := Rect{X: box.X + 44, Y: box.Y + 8, W: max(0, box.W-52), H: max(0, box.H-labelHeight-16)}
	if plot.W < 8 || plot.H < 8 {
		return nil
	}

	domain := chartDomain(node.chart)
	if err := v.chartCartesianAxes(node, box, plot, domain); err != nil {
		return err
	}

	clip := node.clip.Intersect(plot)
	if node.chart.kind == LineChart {
		v.chartLines(node, plot, clip, domain)
	} else {
		v.chartBars(node, plot, clip, domain)
	}

	return nil
}

func (v *View) chartCartesianAxes(node *Node, box, plot Rect, domain ChartRange) error {
	clip := node.clip.Intersect(box)
	axis := v.chartAxisColor(node)

	for index := range 5 {
		fraction := float32(index) / 4

		positionY := plot.Y + plot.H*(1-fraction)
		if !node.chart.options.HideGrid {
			v.chartStroke(node, chartPoint{plot.X, positionY}, chartPoint{plot.X + plot.W, positionY}, clip, v.chartGridColor(node), 1)
		}

		value := domain.Min*(1-float64(fraction)) + domain.Max*float64(fraction)

		bounds := Rect{X: box.X, Y: positionY - 9, W: 36, H: 18}
		if err := v.chartLabel(node, chartNumber(value), bounds, 1, v.theme.Muted); err != nil {
			return err
		}
	}

	baseline := plot.Y + plot.H*(1-chartFraction(0, domain))
	v.chartStroke(node, chartPoint{plot.X, baseline}, chartPoint{plot.X + plot.W, baseline}, clip, axis, 1)
	v.chartStroke(node, chartPoint{plot.X, plot.Y}, chartPoint{plot.X, plot.Y + plot.H}, clip, axis, 1)
	labels := node.chart.data.Labels
	capacity := max(2, int(plot.W/48))

	stride := max(1, int(math.Ceil(float64(len(labels))/float64(capacity))))
	for index, label := range labels {
		if index%stride != 0 && index != len(labels)-1 {
			continue
		}

		positionX := chartSampleX(node, plot, index)

		bounds := Rect{X: positionX - 22, Y: plot.Y + plot.H + 4, W: 44, H: v.chartFontSize(node) * 1.5}
		if err := v.chartLabel(node, label, bounds, 0.5, v.theme.Muted); err != nil {
			return err
		}
	}

	return nil
}

func chartSampleX(node *Node, plot Rect, index int) float32 {
	count := len(node.chart.data.Labels)
	if node.chart.kind == BarChart {
		return plot.X + (float32(index)+0.5)/float32(count)*plot.W
	}

	if count == 1 {
		return plot.X + plot.W/2
	}

	return plot.X + float32(index)/float32(count-1)*plot.W
}

func (v *View) chartLines(node *Node, plot, clip Rect, domain ChartRange) {
	for seriesIndex, series := range node.chart.data.Series {
		color := v.chartColor(node, seriesIndex)
		previous := chartPoint{}

		for index, value := range series.Values {
			point := chartPoint{chartSampleX(node, plot, index), plot.Y + plot.H*(1-chartFraction(value, domain))}
			if index > 0 {
				v.chartStroke(node, previous, point, clip, color, chartLineWidth(node))
			}

			if len(series.Values) <= 32 {
				v.box(node, Rect{X: point.x - 2.5, Y: point.y - 2.5, W: 5, H: 5}, clip, color, Color{}, 2.5, 0)
			}

			previous = point
		}
	}
}

func (v *View) chartBars(node *Node, plot, clip Rect, domain ChartRange) {
	count := len(node.chart.data.Labels)
	groupWidth := plot.W / float32(count)
	width := groupWidth * 0.76 / float32(len(node.chart.data.Series))
	baseline := plot.Y + plot.H*(1-chartFraction(0, domain))

	for seriesIndex, series := range node.chart.data.Series {
		color := v.chartColor(node, seriesIndex)

		for index, value := range series.Values {
			positionY := plot.Y + plot.H*(1-chartFraction(value, domain))
			positionX := plot.X + float32(index)*groupWidth + groupWidth*0.12 + float32(seriesIndex)*width
			v.box(
				node,
				Rect{X: positionX, Y: min(positionY, baseline), W: max(0, width-min(1, width*0.15)), H: abs(positionY - baseline)},
				clip,
				color,
				Color{},
				1,
				0,
			)
		}
	}
}
