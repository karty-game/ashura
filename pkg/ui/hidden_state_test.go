package ui_test

import (
	"encoding/binary"
	"math"
	"testing"
	"time"

	"github.com/karty-game/ashura/internal/uipaint"
	"github.com/karty-game/ashura/pkg/ui"
)

func TestHiddenTabUpdatesDoNotOverwriteVisibleRecords(t *testing.T) {
	t.Parallel()

	progress := ui.NewProgress("Work", 0.2)
	tabs := ui.NewTabs(
		"Sections",
		[]ui.Tab{
			{Key: "first", Label: "First", Content: ui.NewText("Visible content")},
			{Key: "second", Label: "Second", Content: progress},
		},
		"second",
		nil,
	)

	view := surface(t, ui.Column(tabs))
	if err := tabs.SetActiveTab("first"); err != nil {
		t.Fatal(err)
	}

	paint(t, view)
	stats := view.Stats()

	frame, err := view.Frame(time.Second)
	if err != nil {
		t.Fatal(err)
	}

	revision := frame.Revision

	if err := progress.SetProgress(0.8); err != nil {
		t.Fatal(err)
	}

	paint(t, view)

	if view.Stats() != stats || frame.Revision != revision {
		t.Fatal("hidden update disturbed the visible frame", view.Stats(), stats)
	}

	if err := tabs.SetActiveTab("second"); err != nil {
		t.Fatal(err)
	}

	paint(t, view)

	expected := (progress.Bounds().W - 24) * 0.8
	found := false

	for _, batch := range frame.Batches {
		for index := range batch.Count {
			data := frame.Data[(batch.First+index)*uipaint.Stride:]
			width := math.Float32frombits(binary.LittleEndian.Uint32(data[8:]))

			height := math.Float32frombits(binary.LittleEndian.Uint32(data[12:]))
			if height == 8 && math.Abs(float64(width-expected)) < 0.01 {
				found = true
			}
		}
	}

	if !found {
		t.Fatal("hidden progress update was not painted when the panel reopened")
	}
}
