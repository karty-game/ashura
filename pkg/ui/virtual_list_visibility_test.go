package ui_test

import (
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/karty-game/karty-webgpu/pkg/ui"
)

func TestVirtualHiddenUpdatesAndJumpsDeferWorkUntilVisible(t *testing.T) {
	t.Parallel()

	version := 1
	list := ui.NewVirtualList(ui.VirtualListOptions{
		Count: 10000, RowHeight: 40,
		Create: func() *ui.Node { return ui.NewButton("", nil) },
		Bind: func(row *ui.Node, index int) error {
			return row.SetText(strconv.Itoa(index) + " v" + strconv.Itoa(version))
		},
	}).WithStyle(ui.Style{Height: 200})
	view := surface(t, ui.Column(list))
	list.SetHidden(true)
	paint(t, view)

	frame, err := view.Frame(time.Second)
	if err != nil {
		t.Fatal(err)
	}

	before, revision, bindings := view.Stats(), frame.Revision, list.VirtualListStats().Bindings
	version++

	if err = list.RefreshVirtualRows(0, 10000); err != nil {
		t.Fatal(err)
	}

	if err = list.SetVirtualCount(100000); err != nil {
		t.Fatal(err)
	}

	if err = list.ScrollToRow(50000); err != nil {
		t.Fatal(err)
	}

	paint(t, view)

	if view.Stats() != before || frame.Revision != revision || list.VirtualListStats().Bindings != bindings {
		t.Fatal("hidden list changes disturbed the visible frame", before, view.Stats())
	}

	list.SetHidden(false)
	paint(t, view)

	stats := list.VirtualListStats()
	if stats.Count != 100000 || stats.First > 50000 || stats.End <= 50000 || stats.Bindings == bindings {
		t.Fatal("pending hidden changes were not applied", stats)
	}

	for _, semantic := range view.Semantics() {
		if semantic.Parent == list.ID() && semantic.Text != "" && !strings.HasSuffix(semantic.Text, "v2") {
			t.Fatal("revealed row retained obsolete hidden data", semantic)
		}
	}
}
