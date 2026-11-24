package ui_test

import (
	"errors"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/karty-game/ashura/pkg/ui"
	"github.com/karty-game/ashura/pkg/ui/fonts/defaultfont"
)

func TestProgressBoundsAndSilentPaintUpdate(t *testing.T) {
	t.Parallel()

	progress := ui.NewProgress("Download", 0.25)
	button := ui.NewButton("Continue", nil)
	view := surface(t, ui.Column(progress, button))
	before := view.Stats()

	if err := progress.SetProgress(0.75); err != nil {
		t.Fatal(err)
	}

	paint(t, view)

	after := view.Stats()
	if progress.Progress() != 0.75 || before.Layouts != after.Layouts || before.Shapes != after.Shapes || after.Paints != before.Paints+1 {
		t.Fatal("progress update rebuilt unrelated state", before, after)
	}

	frame, err := view.Frame(time.Second)
	if err != nil {
		t.Fatal(err)
	}

	if len(frame.Changes) != 1 {
		t.Fatal("progress update damaged unrelated records", len(frame.Changes))
	}

	for _, invalid := range []float64{-1, 1.01, math.NaN(), math.Inf(1)} {
		if err := progress.SetProgress(invalid); !errors.Is(err, ui.ErrBounds) || progress.Progress() != 0.75 {
			t.Fatal("invalid progress modified value", invalid, progress.Progress(), err)
		}
	}

	if err := button.SetProgress(0.5); !errors.Is(err, ui.ErrBounds) {
		t.Fatal("progress setter accepted a button", err)
	}

	key(view, "Tab")

	if view.Focused() != button {
		t.Fatal("progress was a focus stop")
	}

	foundProgress := false

	for _, semantic := range view.Semantics() {
		if semantic.ID == progress.ID() {
			foundProgress = true
		}

		if semantic.ID == progress.ID() && (semantic.Role != "progressbar" || semantic.Name != "Download" || semantic.Value != 0.75 ||
			semantic.Minimum != 0 || semantic.Maximum != 1 || !semantic.ReadOnly) {
			t.Fatal("incomplete progress semantics", semantic)
		}
	}

	if !foundProgress {
		t.Fatal("progress omitted from semantic projection")
	}
}

func TestSeparatorAndAlertSemantics(t *testing.T) {
	t.Parallel()

	separator := ui.NewSeparator()
	info := ui.NewAlert(ui.InfoAlert, "Network", "Connecting to the server.")
	warning := ui.NewAlert(ui.WarningAlert, "Low storage", "Free some space before downloading.")
	button := ui.NewButton("Continue", nil)
	view := surface(t, ui.Column(info, separator, warning, button))
	key(view, "Tab")

	if view.Focused() != button || separator.Bounds().H != 1 {
		t.Fatal("decorative feedback became interactive", separator.Bounds(), view.Focused())
	}

	roles := map[ui.ID]string{}
	for _, semantic := range view.Semantics() {
		roles[semantic.ID] = semantic.Role
	}

	if roles[info.ID()] != "status" || roles[warning.ID()] != "alert" || roles[separator.ID()] != "separator" {
		t.Fatal("feedback roles lost", roles)
	}

	if !strings.Contains(info.Children()[0].Text(), "Info") || !strings.Contains(warning.Children()[0].Text(), "Warning") {
		t.Fatal("severity communicated only through color")
	}
}

func TestInvalidFeedbackRejectedAtAdmission(t *testing.T) {
	t.Parallel()

	font, err := defaultfont.Load()
	if err != nil {
		t.Fatal(err)
	}

	for _, node := range []*ui.Node{ui.NewProgress("Invalid", math.Inf(1)), ui.NewAlert(ui.AlertTone(255), "Invalid", "message")} {
		if _, err := ui.New(ui.Column(node), font); !errors.Is(err, ui.ErrBounds) {
			t.Fatal("invalid feedback admitted", err)
		}
	}
}
