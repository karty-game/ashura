//nolint:paralleltest // Allocation checks require isolation from parallel test execution.
package ui_test

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/karty-game/karty-webgpu/pkg/ui"
	"github.com/karty-game/karty-webgpu/pkg/ui/fonts/defaultfont"
)

func virtualSurface(tb testing.TB, count int) (*ui.View, *ui.Node, *int) {
	tb.Helper()

	font, err := defaultfont.Load()
	if err != nil {
		tb.Fatal(err)
	}

	bindings := 0
	list := ui.NewVirtualList(ui.VirtualListOptions{Count: count, RowHeight: 40,
		Create: func() *ui.Node { return ui.NewButton("", nil) },
		Bind: func(row *ui.Node, index int) error {
			bindings++

			return row.SetText(fmt.Sprintf("Row %06d", index))
		},
	})

	view, err := ui.New(list, font)
	if err != nil {
		tb.Fatal(err)
	}

	if err = view.Resize(500, 400); err != nil {
		tb.Fatal(err)
	}

	view.SetReducedMotion(true)

	if _, err = view.Frame(time.Second); err != nil {
		tb.Fatal(err)
	}

	return view, list, &bindings
}

func TestVirtualListBoundsWorkByViewport(t *testing.T) {
	view, list, binds := virtualSurface(t, 10000)

	stats := list.VirtualListStats()
	if stats.First != 0 || stats.End != 12 || stats.Mounted != 12 || view.Stats().Nodes != 13 {
		t.Fatal(stats, view.Stats())
	}

	before := *binds

	if err := list.ScrollToRow(1); err != nil {
		t.Fatal(err)
	}

	if _, err := view.Frame(2 * time.Second); err != nil {
		t.Fatal(err)
	}
	// A larger initial window creates at most one extra recycled slot.
	if *binds-before > 13 {
		t.Fatal("unbounded bind", *binds-before)
	}

	before = *binds

	if err := list.ScrollToRow(2); err != nil {
		t.Fatal(err)
	}

	if _, err := view.Frame(3 * time.Second); err != nil {
		t.Fatal(err)
	}

	if *binds-before > 14 {
		t.Fatal("unbounded pool resize", *binds-before)
	}

	before = *binds

	if err := list.ScrollToRow(3); err != nil {
		t.Fatal(err)
	}

	if _, err := view.Frame(4 * time.Second); err != nil {
		t.Fatal(err)
	}

	if *binds-before != 1 {
		t.Fatal("stable pool rebound unchanged rows", *binds-before)
	}

	before = *binds

	if err := list.ScrollToRow(9999); err != nil {
		t.Fatal(err)
	}

	if _, err := view.Frame(5 * time.Second); err != nil {
		t.Fatal(err)
	}

	if *binds-before > 14 || list.VirtualListStats().End != 10000 || list.ScrollOffset() != 399600 {
		t.Fatal(list.VirtualListStats(), list.ScrollOffset())
	}

	if view.Stats().Nodes > 15 || view.Stats().Records > 512 {
		t.Fatal("list resources follow item count", view.Stats())
	}
}

func TestVirtualListOffscreenRefreshIdleAndVisibleRefresh(t *testing.T) {
	view, list, binds := virtualSurface(t, 10000)

	frame, err := view.Frame(time.Second)
	if err != nil {
		t.Fatal(err)
	}

	revision, stats, before := frame.Revision, view.Stats(), *binds

	if err = list.RefreshVirtualRows(9000, 1000); err != nil {
		t.Fatal(err)
	}

	frame, err = view.Frame(2 * time.Second)
	if err != nil {
		t.Fatal(err)
	}

	if frame.Revision != revision || view.Stats() != stats || *binds != before {
		t.Fatal("offscreen update performed frame work")
	}

	if err = list.RefreshVirtualRows(2, 1); err != nil {
		t.Fatal(err)
	}

	if _, err = view.Frame(3 * time.Second); err != nil {
		t.Fatal(err)
	}

	if *binds != before+1 {
		t.Fatal("refresh rebound unrelated rows", *binds-before)
	}

	if got := testing.AllocsPerRun(100, func() { _, _ = view.Frame(4 * time.Second) }); got != 0 {
		t.Fatal("idle allocations", got)
	}

	if err = list.RefreshVirtualRows(0, 10000); err != nil {
		t.Fatal(err)
	}

	before = *binds

	if _, err = view.Frame(5 * time.Second); err != nil {
		t.Fatal(err)
	}

	if *binds-before != list.VirtualListStats().Mounted {
		t.Fatal("full refresh was not viewport bounded")
	}
}

func TestVirtualCountClampAndKeyboardEnds(t *testing.T) {
	view, list, _ := virtualSurface(t, 100000)
	view.SetFocus(list)

	if result := view.Dispatch(ui.Event{Kind: ui.KeyDown, Key: "End"}); !result.Consumed {
		t.Fatal(result)
	}

	if _, err := view.Frame(2 * time.Second); err != nil {
		t.Fatal(err)
	}

	if list.VirtualListStats().End != 100000 {
		t.Fatal("End did not reach full extent", list.VirtualListStats())
	}

	if err := list.SetVirtualCount(3); err != nil {
		t.Fatal(err)
	}

	if _, err := view.Frame(3 * time.Second); err != nil {
		t.Fatal(err)
	}

	if list.ScrollOffset() != 0 || list.VirtualListStats().End != 3 {
		t.Fatal(list.ScrollOffset(), list.VirtualListStats())
	}

	if err := list.SetVirtualCount(0); err != nil {
		t.Fatal(err)
	}

	if _, err := view.Frame(4 * time.Second); err != nil {
		t.Fatal(err)
	}

	if list.VirtualListStats().First != 0 || list.VirtualListStats().End != 0 {
		t.Fatal(list.VirtualListStats())
	}
}

func TestVirtualRecyclingInvalidatesStaleActions(t *testing.T) {
	view, list, _ := virtualSurface(t, 10000)

	row := list.Children()[0]
	for _, candidate := range list.Children() {
		if candidate.Bounds().Y == 0 {
			row = candidate

			break
		}
	}

	oldID := row.ID()
	view.SetFocus(row)
	bounds := row.Bounds()

	result := view.Dispatch(ui.Event{Kind: ui.PointerDown, Pointer: 7, X: bounds.X + 10, Y: bounds.Y + 10})
	if !result.Capture {
		t.Fatal(result)
	}

	if err := list.ScrollToRow(5000); err != nil {
		t.Fatal(err)
	}

	if _, err := view.Frame(2 * time.Second); err != nil {
		t.Fatal(err)
	}

	if view.Lookup(oldID) != nil || view.Focused() == row || view.CapturesPointer() {
		t.Fatal("recycled item retained old interaction identity")
	}

	result = view.Dispatch(ui.Event{Kind: ui.Activate, Target: oldID})
	if result.Consumed {
		t.Fatal("stale action targeted a replacement item")
	}

	for _, semantic := range view.Semantics() {
		if semantic.Parent == list.ID() && (semantic.PosInSet < 4999 || semantic.SetSize != 10000) {
			t.Fatal(semantic)
		}
	}
}

func TestVirtualListHiddenAndClippedPools(t *testing.T) {
	font, err := defaultfont.Load()
	if err != nil {
		t.Fatal(err)
	}

	created := 0
	list := ui.NewVirtualList(ui.VirtualListOptions{Count: 10000, RowHeight: 40, Create: func() *ui.Node {
		created++

		return ui.NewText("Row")
	}, Bind: func(*ui.Node, int) error { return nil }}).
			WithStyle(ui.Style{Height: 400})
	list.SetHidden(true)

	view, err := ui.New(ui.Column(ui.NewPanel().WithStyle(ui.Style{Height: 300}), list).WithStyle(ui.Style{Scroll: true}), font)
	if err != nil {
		t.Fatal(err)
	}

	if err = view.Resize(500, 400); err != nil {
		t.Fatal(err)
	}

	if _, err = view.Frame(time.Second); err != nil {
		t.Fatal(err)
	}

	if created != 0 {
		t.Fatal("hidden list materialized rows", created)
	}

	list.SetHidden(false)

	if _, err = view.Frame(2 * time.Second); err != nil {
		t.Fatal(err)
	}

	if created > 5 {
		t.Fatal("clipped list mounted full viewport", created)
	}
}

func TestVirtualListValidationAndBindingFailure(t *testing.T) {
	font, err := defaultfont.Load()
	if err != nil {
		t.Fatal(err)
	}

	failure := ui.ErrReentrant
	list := ui.NewVirtualList(
		ui.VirtualListOptions{
			Count:     10000,
			RowHeight: 40,
			Create:    func() *ui.Node { return ui.NewText("Row") },
			Bind:      func(*ui.Node, int) error { return failure },
		},
	)

	view, err := ui.New(list, font)
	if err != nil {
		t.Fatal(err)
	}

	if err = view.Resize(500, 400); err != nil {
		t.Fatal(err)
	}

	if _, err = view.Frame(time.Second); !errors.Is(err, failure) {
		t.Fatal(err)
	}

	if err = list.SetVirtualCount(1000001); !errors.Is(err, ui.ErrBounds) {
		t.Fatal(err)
	}

	if err = list.RefreshVirtualRows(9999, 2); !errors.Is(err, ui.ErrBounds) {
		t.Fatal(err)
	}

	if err = list.Add(ui.NewText("Invalid")); !errors.Is(err, ui.ErrBounds) {
		t.Fatal(err)
	}

	invalid := ui.NewVirtualList(ui.VirtualListOptions{}).WithStyle(ui.Style{Height: 400})
	if _, err = ui.New(invalid, font); !errors.Is(err, ui.ErrBounds) {
		t.Fatal("WithStyle erased invalid options", err)
	}
}

func BenchmarkVirtualList(b *testing.B) {
	for _, count := range []int{10000, 100000} {
		for _, mode := range []string{"idle", "scroll", "refresh", "offscreen"} {
			b.Run(fmt.Sprintf("%d/%s", count, mode), func(b *testing.B) {
				view, list, _ := virtualSurface(b, count)
				// Warm the complete pool and repeated visible labels outside the timer.
				for index := range 100 {
					_ = list.ScrollToRow(index)
					_, _ = view.Frame(time.Second)
				}

				b.ReportAllocs()
				b.ResetTimer()

				for index := range b.N {
					switch mode {
					case "scroll":
						_ = list.ScrollToRow(index % 100)
					case "refresh":
						_ = list.RefreshVirtualRows(0, count)
					case "offscreen":
						_ = list.RefreshVirtualRows(count-100, 100)
					}

					if _, err := view.Frame(time.Second); err != nil {
						b.Fatal(err)
					}
				}
			})
		}
	}
}

func BenchmarkVirtualLongScroll(b *testing.B) {
	font, err := defaultfont.Load()
	if err != nil {
		b.Fatal(err)
	}

	list := ui.NewVirtualList(
		ui.VirtualListOptions{
			Count:     100000,
			RowHeight: 40,
			Create:    func() *ui.Node { return ui.NewButton("Static", nil) },
			Bind:      func(*ui.Node, int) error { return nil },
		},
	)

	view, err := ui.New(list, font)
	if err != nil {
		b.Fatal(err)
	}

	if err = view.Resize(500, 400); err != nil {
		b.Fatal(err)
	}

	view.SetReducedMotion(true)

	for index := range 1000 {
		_ = list.ScrollToRow(index)

		if _, err = view.Frame(time.Second); err != nil {
			b.Fatal(err)
		}
	}

	b.ReportAllocs()
	b.ResetTimer()

	for index := range b.N {
		if err = list.ScrollToRow(index % 99000); err != nil {
			b.Fatal(err)
		}

		if _, err = view.Frame(time.Second); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkVirtualColdUniqueScroll(b *testing.B) {
	view, list, _ := virtualSurface(b, 100000)
	b.ReportAllocs()
	b.ResetTimer()

	for index := range b.N {
		// Step beyond the text-cache capacity before wrapping to keep dynamic text cold.
		if err := list.ScrollToRow((index * 17) % 99000); err != nil {
			b.Fatal(err)
		}

		if _, err := view.Frame(time.Second); err != nil {
			b.Fatal(err)
		}
	}
}

func TestVirtualRecyclingCancellationMayRemoveListOrAncestor(t *testing.T) {
	for _, ancestor := range []bool{false, true} {
		t.Run(fmt.Sprintf("ancestor_%t", ancestor), func(t *testing.T) {
			virtualCancellationRemoval(t, ancestor)
		})
	}
}

func TestVirtualRecyclingCompositionCancellationMayRemoveList(t *testing.T) {
	font, err := defaultfont.Load()
	if err != nil {
		t.Fatal(err)
	}

	var list *ui.Node

	calls := 0
	list = ui.NewVirtualList(ui.VirtualListOptions{Count: 10000, RowHeight: 40, Create: func() *ui.Node {
		return ui.NewInput("Name", "Original", "", func(change ui.Change) {
			if !change.Composing {
				calls++

				list.Remove()
			}
		})
	}, Bind: func(*ui.Node, int) error { return nil }}).WithStyle(ui.Style{Height: 400})

	view, err := ui.New(ui.Column(list), font)
	if err != nil {
		t.Fatal(err)
	}

	if err = view.Resize(500, 400); err != nil {
		t.Fatal(err)
	}

	if _, err = view.Frame(time.Second); err != nil {
		t.Fatal(err)
	}

	row := list.Children()[0]
	view.SetFocus(row)
	view.Dispatch(ui.Event{Kind: ui.CompositionStart})

	if err = list.ScrollToRow(5000); err != nil {
		t.Fatal(err)
	}

	if _, err = view.Frame(2 * time.Second); err != nil {
		t.Fatal(err)
	}

	if calls != 1 || view.Stats().Nodes != 1 {
		t.Fatal(calls, view.Stats())
	}
}

func virtualCancellationRemoval(t *testing.T, ancestor bool) {
	t.Helper()

	font, err := defaultfont.Load()
	if err != nil {
		t.Fatal(err)
	}

	var removed *ui.Node

	cancellations := 0
	list := ui.NewVirtualList(ui.VirtualListOptions{Count: 10000, RowHeight: 40, Create: func() *ui.Node {
		row := ui.NewButton("Row", nil)
		row.OnCancel(func() { cancellations++; removed.Remove() })

		return row
	}, Bind: func(*ui.Node, int) error { return nil }}).WithStyle(ui.Style{Height: 400})
	wrapper := ui.Column(list)

	removed = list
	if ancestor {
		removed = wrapper
	}

	view, err := ui.New(ui.Column(wrapper), font)
	if err != nil {
		t.Fatal(err)
	}

	if err = view.Resize(500, 400); err != nil {
		t.Fatal(err)
	}

	if _, err = view.Frame(time.Second); err != nil {
		t.Fatal(err)
	}

	result := view.Dispatch(ui.Event{Kind: ui.PointerDown, Pointer: 7, X: 10, Y: 10})
	if !result.Capture {
		t.Fatal(result)
	}

	if err = list.ScrollToRow(5000); err != nil {
		t.Fatal(err)
	}

	if _, err = view.Frame(2 * time.Second); err != nil {
		t.Fatal(err)
	}

	expected := 1
	if !ancestor {
		expected++
	}

	if cancellations != 1 || view.Stats().Nodes != expected {
		t.Fatal(cancellations, view.Stats())
	}

	if _, err = view.Frame(3 * time.Second); err != nil {
		t.Fatal(err)
	}

	if view.CapturesPointer() {
		t.Fatal("retired capture survived")
	}
}
