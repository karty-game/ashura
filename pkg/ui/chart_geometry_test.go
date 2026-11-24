package ui_test

import (
	"errors"
	"testing"

	"github.com/karty-game/ashura/pkg/ui"
	"github.com/karty-game/ashura/pkg/ui/fonts/defaultfont"
)

func TestInvalidChartConstructionSurvivesStyling(t *testing.T) {
	t.Parallel()

	chart := ui.NewChart(ui.PieChart, "Invalid distribution", ui.ChartData{
		Labels: []string{"Invalid"},
		Series: []ui.ChartSeries{{Values: []float64{-1}}},
	}).WithStyle(ui.Style{Height: 240})

	font, err := defaultfont.Load()
	if err != nil {
		t.Fatal(err)
	}

	if _, err := ui.New(ui.Column(chart), font); !errors.Is(err, ui.ErrBounds) {
		t.Fatal("styling erased invalid chart construction", err)
	}
}

func TestDenseGroupedBarsRetainPositiveWidths(t *testing.T) {
	t.Parallel()

	data := ui.ChartData{Labels: make([]string, ui.MaxChartSamples), Series: make([]ui.ChartSeries, 4)}
	for index := range data.Series {
		data.Series[index].Values = make([]float64, len(data.Labels))
		for sample := range data.Labels {
			data.Series[index].Values[sample] = 1
		}
	}

	chart := ui.NewChart(ui.BarChart, "Dense groups", data)
	if err := chart.SetChartOptions(ui.ChartOptions{HideLegend: true, HideGrid: true}); err != nil {
		t.Fatal(err)
	}

	view := surface(t, ui.Column(chart))
	bars := 0

	for _, record := range buttonRecords(t, view) {
		if record.mode != 0 {
			continue
		}

		bars++

		if record.rect.W <= 0 || record.rect.H <= 0 {
			t.Fatal("dense grouped bar disappeared", record.rect)
		}
	}

	if bars != len(data.Labels)*len(data.Series) {
		t.Fatal("submitted bar count does not match data", bars)
	}
}
