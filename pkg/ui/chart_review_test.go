package ui_test

import (
	"bytes"
	"encoding/binary"
	"errors"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/karty-game/karty-webgpu/internal/uipaint"
	"github.com/karty-game/karty-webgpu/pkg/ui"
)

func reviewChartData() ui.ChartData {
	return ui.ChartData{
		Labels: []string{"A", "B", "C"},
		Series: []ui.ChartSeries{{Name: "Run", Values: []float64{2, 4, 6}}},
	}
}

func TestChartReviewOwnsDataAndOptionsAtomically(t *testing.T) {
	t.Parallel()

	data := reviewChartData()
	chart := ui.NewChart(ui.LineChart, "Scores", data)
	view := surface(t, ui.Column(chart))

	data.Labels[0], data.Series[0].Name, data.Series[0].Values[0] = "changed", "changed", 99
	if got := chart.ChartData(); got.Labels[0] != "A" || got.Series[0].Name != "Run" || got.Series[0].Values[0] != 2 {
		t.Fatal("chart retained caller-owned input", got)
	}

	copyData := chart.ChartData()

	copyData.Labels[0], copyData.Series[0].Values[0] = "changed", 99
	if got := chart.ChartData(); got.Labels[0] != "A" || got.Series[0].Values[0] != 2 {
		t.Fatal("chart getter exposed mutable data", got)
	}

	options := ui.ChartOptions{Range: &ui.ChartRange{Min: 0, Max: 10}, Palette: []ui.Color{ui.Hex(0xff0000)}}
	if err := chart.SetChartOptions(options); err != nil {
		t.Fatal(err)
	}

	options.Range.Max, options.Palette[0] = 100, ui.Hex(0x00ff00)

	got := chart.ChartOptions()
	if got.Range.Max != 10 || got.Palette[0] != ui.Hex(0xff0000) {
		t.Fatal("chart retained caller-owned options", got)
	}

	got.Range.Max, got.Palette[0] = 100, ui.Hex(0x00ff00)
	if current := chart.ChartOptions(); current.Range.Max != 10 || current.Palette[0] != ui.Hex(0xff0000) {
		t.Fatal("chart getter exposed mutable options", current)
	}

	invalid := reviewChartData()

	invalid.Series[0].Values[1] = math.NaN()
	if err := chart.SetChartData(invalid); !errors.Is(err, ui.ErrBounds) || chart.ChartData().Series[0].Values[1] != 4 {
		t.Fatal("invalid data changed mounted chart", err, chart.ChartData())
	}

	if err := chart.SetChartOptions(ui.ChartOptions{Range: &ui.ChartRange{Min: 10, Max: 0}}); !errors.Is(err, ui.ErrBounds) ||
		chart.ChartOptions().Range.Max != 10 {
		t.Fatal("invalid options changed chart", err)
	}

	paint(t, view)
}

func TestChartReviewRepaintsOnlyChangedChartAndThenIdles(t *testing.T) {
	t.Parallel()

	chart := ui.NewChart(ui.LineChart, "Scores", reviewChartData())
	if err := chart.SetChartOptions(ui.ChartOptions{Range: &ui.ChartRange{Min: 0, Max: 10}}); err != nil {
		t.Fatal(err)
	}

	view := surface(t, ui.Column(chart, ui.NewButton("Continue", nil)))
	before := view.Stats()

	frame, err := view.Frame(time.Second)
	if err != nil {
		t.Fatal(err)
	}

	dataBefore := bytes.Clone(frame.Data)
	data := reviewChartData()

	data.Series[0].Values = []float64{3, 5, 7}
	if err := chart.SetChartData(data); err != nil {
		t.Fatal(err)
	}

	paint(t, view)

	after := view.Stats()
	if after.Paints != before.Paints+1 || after.Layouts != before.Layouts || after.Measures != before.Measures {
		t.Fatal("data update rebuilt unrelated UI", before, after)
	}

	if len(frame.Changes) != 1 {
		t.Fatal("chart update rewrote unrelated blocks", frame.Changes)
	}

	change := frame.Changes[0]
	if !bytes.Equal(dataBefore[:change.Offset], frame.Data[:change.Offset]) ||
		!bytes.Equal(dataBefore[change.Offset+change.Size:], frame.Data[change.Offset+change.Size:len(dataBefore)]) {
		t.Fatal("chart update changed unrelated retained records")
	}

	revision := frame.Revision

	if _, err := view.Frame(2 * time.Second); err != nil {
		t.Fatal(err)
	}

	if view.Stats() != after || frame.Revision != revision {
		t.Fatal("unchanged chart did not return to retained idle")
	}
}

func TestChartReviewDegenerateInputsAndClippedGeometry(t *testing.T) {
	t.Parallel()

	for _, example := range []struct {
		name string
		kind ui.ChartKind
		data ui.ChartData
	}{
		{"one line sample", ui.LineChart, ui.ChartData{Labels: []string{"Now"}, Series: []ui.ChartSeries{{Name: "Frame", Values: []float64{16}}}}},
		{"constant line", ui.LineChart, ui.ChartData{Labels: []string{"A", "B"}, Series: []ui.ChartSeries{{Values: []float64{5, 5}}}}},
		{"signed bars", ui.BarChart, ui.ChartData{Labels: []string{"A", "B"}, Series: []ui.ChartSeries{{Values: []float64{-2, 4}}}}},
		{"extreme signed line", ui.LineChart, ui.ChartData{Labels: []string{"A", "B"}, Series: []ui.ChartSeries{{Values: []float64{-1e308, 1e308}}}}},
		{"extreme pie", ui.PieChart, ui.ChartData{Labels: []string{"A", "B"}, Series: []ui.ChartSeries{{Values: []float64{1e308, 1e308}}}}},
		{"zero pie", ui.PieChart, ui.ChartData{Labels: []string{"A", "B"}, Series: []ui.ChartSeries{{Values: []float64{0, 0}}}}},
		{"single slice", ui.PieChart, ui.ChartData{Labels: []string{"A"}, Series: []ui.ChartSeries{{Values: []float64{1}}}}},
		{"zero radar", ui.RadarChart, ui.ChartData{Labels: []string{"A", "B", "C"}, Series: []ui.ChartSeries{{Values: []float64{0, 0, 0}}}}},
	} {
		t.Run(example.name, func(t *testing.T) {
			t.Parallel()

			chart := ui.NewChart(example.kind, "Test chart", example.data).WithStyle(ui.Style{Height: 180})

			view := surface(t, ui.Column(chart))
			if err := view.Resize(190, 180); err != nil {
				t.Fatal(err)
			}

			paint(t, view)
			assertFiniteChartRecords(t, view)
		})
	}
}

func assertFiniteChartRecords(t *testing.T, view *ui.View) {
	t.Helper()

	frame, err := view.Frame(time.Second)
	if err != nil {
		t.Fatal(err)
	}

	for _, batch := range frame.Batches {
		for index := range batch.Count {
			data := frame.Data[(batch.First+index)*uipaint.Stride:]
			for offset := 0; offset < uipaint.Stride; offset += 4 {
				value := math.Float32frombits(binary.LittleEndian.Uint32(data[offset:]))
				if math.IsNaN(float64(value)) || math.IsInf(float64(value), 0) {
					t.Fatal("chart emitted nonfinite GPU geometry", index, offset, value)
				}
			}

			for axis := range 4 {
				clip := math.Float32frombits(binary.LittleEndian.Uint32(data[16+axis*4:]))
				if clip < 0 || (axis%2 == 0 && clip > frame.Width) || (axis%2 == 1 && clip > frame.Height) {
					t.Fatal("chart clip escaped the UI viewport", axis, clip, frame.Width, frame.Height)
				}
			}
		}
	}
}

func TestChartReviewSemanticSummaryAndHiddenUpdates(t *testing.T) {
	t.Parallel()

	chart := ui.NewChart(ui.BarChart, "Scores", reviewChartData())
	tabs := ui.NewTabs("Sections", []ui.Tab{
		{Key: "chart", Label: "Chart", Content: chart},
		{Key: "other", Label: "Other", Content: ui.NewButton("Continue", nil)},
	}, "chart", nil)
	view := surface(t, ui.Column(tabs))
	found := false

	for _, item := range view.Semantics() {
		if item.ID == chart.ID() {
			found = item.Role == "img" && item.ReadOnly && item.Name == "Scores" &&
				strings.Contains(item.Text+item.Description, "Run") && strings.Contains(item.Text+item.Description, "A")
		}
	}

	if !found {
		t.Fatal("chart did not provide a named readonly data summary", view.Semantics())
	}

	if err := tabs.SetActiveTab("other"); err != nil {
		t.Fatal(err)
	}

	paint(t, view)
	before := view.Stats()
	data := reviewChartData()

	data.Series[0].Values[0] = 9
	if err := chart.SetChartData(data); err != nil {
		t.Fatal(err)
	}

	paint(t, view)

	if view.Stats() != before {
		t.Fatal("hidden chart update disturbed visible records", before, view.Stats())
	}

	if err := tabs.SetActiveTab("chart"); err != nil {
		t.Fatal(err)
	}

	paint(t, view)

	if chart.ChartData().Series[0].Values[0] != 9 {
		t.Fatal("hidden data update was lost")
	}

	key(view, "Tab")

	if view.Focused() == chart {
		t.Fatal("readonly chart became a keyboard focus stop")
	}
}
