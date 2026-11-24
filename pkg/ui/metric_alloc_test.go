package ui_test

import (
	"strconv"
	"testing"
	"time"

	"github.com/karty-game/ashura/pkg/ui"
	"github.com/karty-game/ashura/pkg/ui/fonts/defaultfont"
)

const metricValueCount = 2048

func changingMetricFixture(tb testing.TB) (*ui.View, *ui.Node, []string) {
	tb.Helper()

	font, err := defaultfont.Load()
	if err != nil {
		tb.Fatal(err)
	}

	label := ui.NewText("0.00 ms").WithStyle(ui.Style{FontSize: 12})

	view, err := ui.New(ui.Column(label), font)
	if err != nil {
		tb.Fatal(err)
	}

	if err := view.Resize(360, 720); err != nil {
		tb.Fatal(err)
	}

	values := make([]string, metricValueCount)
	for index := range values {
		values[index] = strconv.FormatFloat(float64(index)/100, 'f', 2, 64) + " ms"
	}
	// More distinct values than the LRU can hold keeps the measured prefix on
	// cache misses, including after every complete benchmark cycle.
	for _, value := range values {
		if err := label.SetText(value); err != nil {
			tb.Fatal(err)
		}

		if _, err := view.Frame(time.Second); err != nil {
			tb.Fatal(err)
		}
	}

	return view, label, values
}

//nolint:paralleltest // AllocsPerRun must remain isolated on Go 1.27.
func TestChangingMetricTextKeepsShapingAllocationsBounded(t *testing.T) {
	view, label, values := changingMetricFixture(t)
	index := 0
	allocations := testing.AllocsPerRun(100, func() {
		if err := label.SetText(values[index]); err != nil {
			t.Fatal(err)
		}

		if _, err := view.Frame(time.Second); err != nil {
			t.Fatal(err)
		}

		index++
	})

	const maximumAllocations = 8
	if allocations > maximumAllocations {
		t.Fatalf("uncached metric update allocated %g times, maximum %d", allocations, maximumAllocations)
	}
}

func BenchmarkChangingMetricText(b *testing.B) {
	view, label, values := changingMetricFixture(b)
	b.ReportAllocs()
	b.ResetTimer()

	index := 0
	for b.Loop() {
		if err := label.SetText(values[index%len(values)]); err != nil {
			b.Fatal(err)
		}

		if _, err := view.Frame(time.Second); err != nil {
			b.Fatal(err)
		}

		index++
	}
}
