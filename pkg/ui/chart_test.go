package ui_test

import (
	"errors"
	"math"
	"testing"
	"time"

	"github.com/karty-game/ashura/pkg/ui"
)

func TestChartRejectsInvalidSnapshotsAndDomainsAtomically(t *testing.T) {
	t.Parallel()

	valid := ui.ChartData{Labels: []string{"A", "B", "C"}, Series: []ui.ChartSeries{{Name: "Run", Values: []float64{1, 2, 3}}}}
	for _, kind := range []ui.ChartKind{ui.LineChart, ui.BarChart, ui.PieChart, ui.RadarChart} {
		chart := ui.NewChart(kind, "Stats", valid)
		view := surface(t, ui.Column(chart))
		before := view.Stats()
		invalid := chart.ChartData()

		invalid.Series[0].Values[1] = math.NaN()
		if err := chart.SetChartData(invalid); !errors.Is(err, ui.ErrBounds) || chart.ChartData().Series[0].Values[1] != 2 {
			t.Fatal("invalid snapshot changed chart", kind, err)
		}

		if err := chart.SetChartOptions(ui.ChartOptions{Range: &ui.ChartRange{Min: 2, Max: 1}}); !errors.Is(err, ui.ErrBounds) {
			t.Fatal("reversed domain accepted", kind, err)
		}

		if err := chart.SetChartData(valid); err != nil {
			t.Fatal(err)
		}

		if _, err := view.Frame(2 * time.Second); err != nil {
			t.Fatal(err)
		}

		if view.Stats() != before {
			t.Fatal("rejected or unchanged snapshots repainted the chart", kind, before, view.Stats())
		}
	}
}

func TestChartRadialBoundsAndNonnegativeData(t *testing.T) {
	t.Parallel()

	for _, kind := range []ui.ChartKind{ui.PieChart, ui.RadarChart} {
		chart := ui.NewChart(kind, "Stats", ui.ChartData{})

		invalid := ui.ChartData{Labels: []string{"A", "B", "C"}, Series: []ui.ChartSeries{{Values: []float64{1, -1, 3}}}}
		if err := chart.SetChartData(invalid); !errors.Is(err, ui.ErrBounds) {
			t.Fatal("negative radial value accepted", kind, err)
		}
	}

	radar := ui.NewChart(ui.RadarChart, "Stats", ui.ChartData{})
	if err := radar.SetChartData(
		ui.ChartData{Labels: []string{"A", "B"}, Series: []ui.ChartSeries{{Values: []float64{1, 2}}}},
	); !errors.Is(
		err,
		ui.ErrBounds,
	) {
		t.Fatal("radar accepted fewer than three axes", err)
	}

	pie := ui.NewChart(ui.PieChart, "Stats", ui.ChartData{})
	if err := pie.SetChartOptions(ui.ChartOptions{Range: &ui.ChartRange{Min: 0, Max: 10}}); !errors.Is(err, ui.ErrBounds) {
		t.Fatal("pie accepted range", err)
	}
}
