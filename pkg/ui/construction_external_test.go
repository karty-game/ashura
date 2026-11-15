package ui_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/karty-game/karty-webgpu/pkg/ui"
	"github.com/karty-game/karty-webgpu/pkg/ui/fonts/defaultfont"
)

func TestConstructionPreservesFirstFailureThroughConfiguration(t *testing.T) {
	t.Parallel()

	font, err := defaultfont.Load()
	if err != nil {
		t.Fatal(err)
	}

	child := ui.NewText("Child")

	cases := []struct {
		name string
		node *ui.Node
	}{
		{"selection", ui.NewSelect("Mode", []ui.Option{{Key: "valid", Label: "Valid"}}, "missing", nil)},
		{"duplicate child", ui.Column(child, child)},
		{"style", ui.NewButton("Action", nil).WithStyle(ui.Style{Width: -1})},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			first := test.node.Err()
			if !errors.Is(first, ui.ErrBounds) {
				t.Fatal("construction did not retain the initial failure", first)
			}

			test.node.WithStyle(ui.Style{Width: 100}).WithTooltip("Valid help").WithLabel("Valid name", "")

			if !errors.Is(test.node.Err(), first) {
				t.Fatal("later configuration replaced the original construction failure", test.node.Err())
			}

			_, err := ui.New(ui.Column(test.node), font)
			if !errors.Is(err, first) || !strings.Contains(err.Error(), "children[0]") {
				t.Fatal("mount lost the first error or child path", err)
			}
		})
	}
}

func TestConstructionHelpersReportFailuresAtMount(t *testing.T) {
	t.Parallel()

	font, err := defaultfont.Load()
	if err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name string
		node *ui.Node
	}{
		{"WithTooltip", ui.NewButton("Action", nil).WithTooltip(string([]byte{0xff}))},
		{"WithLabel", ui.NewButton("Action", nil).WithLabel(string([]byte{0xff}), "")},
		{"WithButtonVariant", ui.NewText("Text").WithButtonVariant(ui.PrimaryButton)},
		{"WithButtonVisual", ui.NewButton("Action", nil).WithButtonVisual(ui.ButtonVisual{Size: -1})},
		{"WithPanelBackground", ui.Column().WithPanelBackground(ui.PanelBackground{Fit: ui.CoverImage})},
		{"WithChartOptions", ui.NewChart(ui.LineChart, "Chart", ui.ChartData{}).WithChartOptions(ui.ChartOptions{LineWidth: -1})},
		{"WithHelpText", ui.NewText("Text").WithHelpText("Hint")},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			_, err := ui.New(test.node.WithStyle(ui.Style{Width: 100}), font)
			if !errors.Is(err, ui.ErrBounds) || !strings.Contains(err.Error(), test.name) {
				t.Fatal("mount did not identify failed construction operation", err)
			}
		})
	}
}

func TestConstructionHelpersAndLiveErrorsRemainSeparate(t *testing.T) {
	t.Parallel()

	button := ui.NewButton("Save", nil).
		WithLabel("Save settings", "Writes your choices").
		WithTooltip("Save current settings").
		WithButtonVariant(ui.PrimaryButton).
		WithButtonVisual(ui.ButtonVisual{Icon: "+", Size: 20})
	field := ui.NewField("Name", ui.NewInput("Name", "Explorer", "", nil)).WithHelpText("Visible to other players")
	view := surface(t, ui.Column(button, field))

	if button.Err() != nil || button.Tooltip() != "Save current settings" || button.ButtonVariant() != ui.PrimaryButton ||
		button.ButtonVisual().Icon != "+" {
		t.Fatal("valid construction configuration was not retained", button.Err())
	}

	before := button.Style()

	err := button.SetStyle(ui.Style{Width: -1})
	if !errors.Is(err, ui.ErrBounds) || button.Style() != before || button.Err() != nil {
		t.Fatal("live error mutated style or became a retained construction error", err)
	}

	button.WithTooltip(string([]byte{0xff}))

	if !errors.Is(button.Err(), ui.ErrBounds) || button.Tooltip() != "Save current settings" {
		t.Fatal("mounted fluent misuse silently lost its error", button.Err())
	}

	if err := button.SetTooltip("Updated help"); err != nil {
		t.Fatal(err)
	}

	paint(t, view)
}

func TestValidationErrorIdentifiesOperationAndField(t *testing.T) {
	t.Parallel()

	button := ui.NewButton("Save", nil)
	err := button.SetStyle(ui.Style{Width: -12})

	var detail *ui.ValidationError

	if !errors.Is(err, ui.ErrBounds) || !errors.As(err, &detail) || detail.Op != "SetStyle" || detail.Field != "Width" ||
		detail.Value != float32(-12) || detail.Reason == "" {
		t.Fatal("validation error lacks useful details", err)
	}

	err = button.SetText(strings.Repeat("private text", ui.MaxTextBytes))
	if !errors.As(err, &detail) || detail.Op != "SetText" || strings.Contains(err.Error(), "private text") {
		t.Fatal("text validation exposed content or lost operation context", err)
	}
}
