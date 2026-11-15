package ui

import (
	"math"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"
)

type ChartKind uint8

const (
	LineChart ChartKind = iota
	BarChart
	PieChart
	RadarChart
)

const (
	MaxChartSamples    = 128
	MaxChartSeries     = 8
	MaxChartLabelBytes = 128
)

type ChartSeries struct {
	Name   string
	Values []float64
	Color  Color
}

type ChartData struct {
	Labels []string
	Series []ChartSeries
}

type ChartRange struct{ Min, Max float64 }

// ChartOptions controls presentation. Nil Range chooses a zero-inclusive domain.
// Pie charts use nonnegative proportions; radar ranges always start at zero.
type ChartOptions struct {
	Range                *ChartRange
	Palette              []Color
	HideLegend, HideGrid bool
	LineWidth            float32
}

type chartState struct {
	kind    ChartKind
	data    ChartData
	options ChartOptions
	summary string
}

// NewChart creates a read-only retained chart. SetLabel may add a description;
// the semantic snapshot also exposes a bounded textual data summary.
func NewChart(kind ChartKind, label string, data ChartData) *Node {
	node := &Node{kind: Chart, label: label, readOnly: true, chart: &chartState{kind: kind}}

	node.constructionErr = node.SetChartData(data)
	if node.constructionErr != nil {
		node.chart = nil
	}

	return node
}

// SetChartData validates and copies a complete data snapshot atomically.
// It repaints this chart without remeasuring the surrounding interface.
func (node *Node) SetChartData(data ChartData) error {
	if node.kind != Chart || node.chart == nil || validateChartData(node.chart.kind, data) != nil {
		return ErrBounds
	}

	if node.chart.summary != "" && equalChartData(node.chart.data, data) {
		return nil
	}

	node.chart.data = copyChartData(data)
	node.chart.summary = chartSummary(data)
	node.damage(false)

	return nil
}

func (node *Node) ChartData() ChartData {
	if node.chart == nil {
		return ChartData{}
	}

	return copyChartData(node.chart.data)
}

func (node *Node) SetChartOptions(options ChartOptions) error {
	if node.kind != Chart || node.chart == nil || validateChartOptions(node.chart.kind, options) != nil {
		return ErrBounds
	}

	if equalChartOptions(node.chart.options, options) {
		return nil
	}

	node.chart.options = copyChartOptions(options)
	node.damage(false)

	return nil
}

func (node *Node) ChartOptions() ChartOptions {
	if node.chart == nil {
		return ChartOptions{}
	}

	return copyChartOptions(node.chart.options)
}

func validateChartNode(node *Node) error {
	if node.kind != Chart {
		return nil
	}

	if node.chart == nil || validateChartData(node.chart.kind, node.chart.data) != nil {
		return ErrBounds
	}

	return validateChartOptions(node.chart.kind, node.chart.options)
}

func validateChartData(kind ChartKind, data ChartData) error {
	if kind > RadarChart || len(data.Labels) > MaxChartSamples || len(data.Series) > MaxChartSeries {
		return ErrBounds
	}

	if len(data.Series) == 0 && len(data.Labels) == 0 {
		return nil
	}

	if len(data.Series) == 0 || len(data.Labels) == 0 {
		return ErrBounds
	}

	if kind == PieChart && (len(data.Series) != 1 || len(data.Labels) > 32) {
		return ErrBounds
	}

	if kind == RadarChart && (len(data.Labels) < 3 || len(data.Labels) > 16) {
		return ErrBounds
	}

	for _, label := range data.Labels {
		if !validChartLabel(label) {
			return ErrBounds
		}
	}

	for _, series := range data.Series {
		if !validChartSeries(kind, series, len(data.Labels)) {
			return ErrBounds
		}
	}

	return nil
}

func validChartSeries(kind ChartKind, series ChartSeries, count int) bool {
	if !validChartLabel(series.Name) || len(series.Values) != count || !validChartColor(series.Color) {
		return false
	}

	for _, value := range series.Values {
		if math.IsNaN(value) || math.IsInf(value, 0) || ((kind == PieChart || kind == RadarChart) && value < 0) {
			return false
		}
	}

	return true
}

func validChartLabel(label string) bool {
	return len(label) <= MaxChartLabelBytes && utf8.ValidString(label) && !strings.ContainsAny(label, "\r\n\t\x00")
}

func validChartColor(color Color) bool {
	for _, channel := range color {
		if !finite(channel) || channel < 0 || channel > 1 {
			return false
		}
	}

	return true
}

func validateChartOptions(kind ChartKind, options ChartOptions) error {
	if !finite(options.LineWidth) || options.LineWidth < 0 || options.LineWidth > 16 || len(options.Palette) > 32 {
		return ErrBounds
	}

	for _, color := range options.Palette {
		if !validChartColor(color) {
			return ErrBounds
		}
	}

	if options.Range == nil {
		return nil
	}

	domain := *options.Range
	if math.IsNaN(domain.Min) || math.IsNaN(domain.Max) || math.IsInf(domain.Min, 0) || math.IsInf(domain.Max, 0) ||
		domain.Min >= domain.Max {
		return ErrBounds
	}

	if kind == PieChart || (kind == RadarChart && domain.Min != 0) || (kind == BarChart && (domain.Min > 0 || domain.Max < 0)) {
		return ErrBounds
	}

	return nil
}

func copyChartData(data ChartData) ChartData {
	result := ChartData{Labels: append([]string(nil), data.Labels...), Series: make([]ChartSeries, len(data.Series))}
	for index, series := range data.Series {
		result.Series[index] = series
		result.Series[index].Values = append([]float64(nil), series.Values...)
	}

	return result
}

func copyChartOptions(options ChartOptions) ChartOptions {
	options.Palette = append([]Color(nil), options.Palette...)
	if options.Range != nil {
		domain := *options.Range
		options.Range = &domain
	}

	return options
}

func chartSummary(data ChartData) string {
	if len(data.Series) == 0 {
		return "No data"
	}

	var result strings.Builder

	for index, series := range data.Series {
		name := series.Name
		if name == "" {
			name = "Series " + strconv.Itoa(index+1)
		}

		if result.Len()+len(name)+2 > MaxTextBytes-4 {
			break
		}

		if result.Len() > 0 {
			result.WriteString("; ")
		}

		result.WriteString(name)
		result.WriteString(": ")

		for sample, value := range series.Values {
			entry := data.Labels[sample] + " " + strconv.FormatFloat(value, 'g', 6, 64)
			if result.Len()+len(entry)+2 > MaxTextBytes-4 {
				result.WriteString("...")

				return result.String()
			}

			if sample > 0 {
				result.WriteString(", ")
			}

			result.WriteString(entry)
		}
	}

	return result.String()
}

func equalChartData(first, second ChartData) bool {
	if !slices.Equal(first.Labels, second.Labels) || len(first.Series) != len(second.Series) {
		return false
	}

	for index, series := range first.Series {
		other := second.Series[index]
		if series.Name != other.Name || series.Color != other.Color || !slices.Equal(series.Values, other.Values) {
			return false
		}
	}

	return true
}

func equalChartOptions(first, second ChartOptions) bool {
	if first.HideLegend != second.HideLegend || first.HideGrid != second.HideGrid || first.LineWidth != second.LineWidth ||
		!slices.Equal(first.Palette, second.Palette) {
		return false
	}

	if first.Range == nil || second.Range == nil {
		return first.Range == second.Range
	}

	return *first.Range == *second.Range
}
