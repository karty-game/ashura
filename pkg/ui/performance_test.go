package ui_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/karty-game/karty-webgpu/pkg/ui"
	"github.com/karty-game/karty-webgpu/pkg/ui/fonts/defaultfont"
)

func BenchmarkUIFrame(b *testing.B) {
	for _, scenario := range []string{"idle", "slider", "scroll"} {
		b.Run(scenario, func(b *testing.B) {
			view, slider := benchmarkSurface(b)

			b.ReportAllocs()
			b.ResetTimer()

			for index := range b.N {
				frameAt := time.Second

				switch scenario {
				case "slider":
					if err := slider.SetValue(float64(index % 100)); err != nil {
						b.Fatal(err)
					}
				case "scroll":
					frameAt += time.Duration(index) * (time.Second / 60)

					delta := float32(10)
					if index%800 >= 400 {
						delta = -delta
					}

					view.Dispatch(ui.Event{Kind: ui.Wheel, X: 250, Y: 200, DeltaY: delta})
				}

				if _, err := view.Frame(frameAt); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func benchmarkSurface(b *testing.B) (*ui.View, *ui.Node) {
	b.Helper()

	font, err := defaultfont.Load()
	if err != nil {
		b.Fatal(err)
	}

	slider := ui.NewSlider("Value", 0, 100, 1, 50, nil)

	children := make([]*ui.Node, 0, 101)

	children = append(children, slider)
	for index := range 100 {
		children = append(children, ui.NewButton(fmt.Sprintf("Item %03d", index), nil))
	}

	view, err := ui.New(ui.Column(children...).WithStyle(ui.Style{Scroll: true}), font)
	if err != nil {
		b.Fatal(err)
	}

	if err := view.Resize(500, 400); err != nil {
		b.Fatal(err)
	}

	if _, err := view.Frame(time.Second); err != nil {
		b.Fatal(err)
	}

	return view, slider
}
