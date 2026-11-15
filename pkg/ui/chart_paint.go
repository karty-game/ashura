//nolint:mnd // Chart gutters, label spacing and line widths are logical-pixel presentation metrics.
package ui

import (
	"math"
	"strconv"
)

func (v *View) paintChart(node *Node, box Rect) error {
	if box.W <= 0 || box.H <= 0 {
		return nil
	}

	state := node.chart
	if state == nil || len(state.data.Series) == 0 {
		return v.chartEmpty(node, box)
	}

	if state.kind == PieChart && chartPieTotal(state.data.Series[0].Values) == 0 {
		return v.chartEmpty(node, box)
	}

	if !state.options.HideLegend {
		var err error

		box, err = v.chartLegend(node, box)
		if err != nil {
			return err
		}
	}

	switch state.kind {
	case LineChart, BarChart:
		return v.paintCartesianChart(node, box)
	case PieChart:
		v.paintPieChart(node, box)
	case RadarChart:
		return v.paintRadarChart(node, box)
	}

	return nil
}

func (v *View) chartEmpty(node *Node, box Rect) error {
	return v.chartLabel(node, "No data", box, 0.5, v.theme.Muted)
}

func (v *View) chartFontSize(node *Node) float32 {
	if node.style.FontSize > 0 {
		return node.style.FontSize
	}

	return 12
}

func (v *View) chartLabel(node *Node, text string, bounds Rect, align float32, color Color) error {
	if bounds.W <= 0 || bounds.H <= 0 {
		return nil
	}

	layoutData, err := v.shape(text, v.chartFontSize(node), max(1, bounds.W), false)
	if err != nil {
		return err
	}

	width := min(textExtent(layoutData), bounds.W)
	positionX := bounds.X + (bounds.W-width)*align
	positionY := bounds.Y + (bounds.H-layoutData.height)/2

	ink := glyphBounds(layoutData)
	if ink.H > 0 {
		positionY = bounds.Y + (bounds.H-ink.H)/2 - ink.Y
	}

	color = v.part(node, ChartLabelPart, PartStyle{Fill: color}).Fill
	v.paintLayout(node, layoutData, positionX, positionY, node.clip.Intersect(node.rect).Intersect(bounds), color)

	return nil
}

func (v *View) chartLegend(node *Node, box Rect) (Rect, error) {
	state := node.chart

	count := len(state.data.Series)
	if state.kind == PieChart {
		count = len(state.data.Labels)
	}

	columns := max(1, min(count, int(box.W/140)))
	rows := (count + columns - 1) / columns
	lineHeight := v.chartFontSize(node)*1.5 + 4
	height := min(float32(rows)*lineHeight, box.H*0.4)
	availableRows := max(1, int(height/lineHeight))
	// Preserve plot space: long legends show a bounded prefix and a final count.
	limit := min(count, columns*availableRows)

	cellWidth := box.W / float32(columns)
	for index := range limit {
		cell := Rect{
			X: box.X + float32(index%columns)*cellWidth,
			Y: box.Y + box.H - height + float32(index/columns)*lineHeight,
			W: cellWidth,
			H: lineHeight,
		}

		var label string
		if state.kind == PieChart {
			label = state.data.Labels[index] + " · " + chartNumber(state.data.Series[0].Values[index])
		} else {
			label = chartSeriesName(state.data.Series[index], index)
		}

		if index == limit-1 && limit < count {
			label = "+" + strconv.Itoa(count-index) + " more"
		}

		color := v.chartColor(node, index)
		v.box(node, Rect{X: cell.X, Y: cell.Y + (lineHeight-8)/2, W: 8, H: 8}, node.clip.Intersect(box), color, Color{}, 2, 0)
		cell.X += 14

		cell.W = max(0, cell.W-20)
		if err := v.chartLabel(node, label, cell, 0, v.theme.Muted); err != nil {
			return box, err
		}
	}

	box.H = max(0, box.H-height-8)

	return box, nil
}

func chartSeriesName(series ChartSeries, index int) string {
	if series.Name != "" {
		return series.Name
	}

	return "Series " + strconv.Itoa(index+1)
}

func (v *View) chartColor(node *Node, index int) Color {
	state := node.chart

	color := Color{}
	if state.kind != PieChart {
		color = state.data.Series[index].Color
	}

	if color == (Color{}) {
		palette := state.options.Palette
		if len(palette) > 0 {
			color = palette[index%len(palette)]
		} else {
			colors := [8]Color{
				v.theme.Accent,
				v.theme.Info,
				v.theme.Warning,
				v.theme.Danger,
				Hex(0xc6a0f6),
				Hex(0x8bd5ca),
				Hex(0xf5a97f),
				Hex(0xb8c0e0),
			}
			color = colors[index%len(colors)]
		}
	}

	if inheritedDisabled(node) {
		color[3] *= 0.4
	}

	return color
}

func (v *View) chartGridColor(node *Node) Color {
	color := v.theme.Border
	color[3] *= 0.45

	return v.part(node, ChartGridPart, PartStyle{Fill: color}).Fill
}

func (v *View) chartAxisColor(node *Node) Color {
	return v.part(node, ChartAxisPart, PartStyle{Fill: v.theme.Border}).Fill
}

func chartLineWidth(node *Node) float32 {
	if node.chart.options.LineWidth > 0 {
		return node.chart.options.LineWidth
	}

	return 2
}

func chartNumber(value float64) string {
	magnitude := math.Abs(value)
	if magnitude >= 1e6 || (magnitude > 0 && magnitude < 0.01) {
		return strconv.FormatFloat(value, 'g', 3, 64)
	}

	if magnitude >= 1000 {
		return strconv.FormatFloat(value/1000, 'f', 1, 64) + "k"
	}

	return strconv.FormatFloat(value, 'g', 3, 64)
}

func chartDomain(state *chartState) ChartRange {
	if state.options.Range != nil {
		return *state.options.Range
	}

	domain := ChartRange{}

	for _, series := range state.data.Series {
		for _, value := range series.Values {
			domain.Min = min(domain.Min, value)
			domain.Max = max(domain.Max, value)
		}
	}

	if domain.Min == domain.Max {
		domain.Max = domain.Min + 1
	}

	return domain
}

func chartFraction(value float64, domain ChartRange) float32 {
	scale := max(math.Abs(domain.Min), math.Abs(domain.Max))
	if scale == 0 {
		return 0
	}

	fraction := (value/scale - domain.Min/scale) / (domain.Max/scale - domain.Min/scale)

	return float32(max(0, min(1, fraction)))
}

func chartPieTotal(values []float64) float64 {
	maximum := float64(0)
	for _, value := range values {
		maximum = max(maximum, value)
	}

	if maximum == 0 {
		return 0
	}

	total := float64(0)
	for _, value := range values {
		total += value / maximum
	}

	return total
}
