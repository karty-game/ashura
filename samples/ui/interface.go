//nolint:mnd // Sample layout uses preferred sizes and spacing rather than absolute positions.
package main

import (
	"unicode/utf8"

	"github.com/karty-game/karty-webgpu/pkg/ui"
)

func buildInterface(art artwork, check func(error)) (*ui.Node, func(*ui.View)) {
	theme := ui.DarkTheme()
	status := ui.NewText("Ready. Every control below is painted with WebGPU.").WithStyle(ui.Style{FontSize: 14, Foreground: theme.Muted})
	update := func(text string) { check(status.SetText(statusMessage(text))) }

	var view *ui.View

	closeButton := ui.NewButton("Close", func() { view.PopModal() })
	closeButton.WithButtonVariant(ui.PrimaryButton)
	dialog := ui.Column(
		ui.NewText("A real focus scope").WithStyle(ui.Style{FontSize: 22}),
		ui.NewText("Tab stays inside. Escape or Close returns focus to the opener."),
		ui.NewInput("Dialog note", "", "A short note", nil),
		ui.NewCheckbox("Remember this choice", true, nil),
		ui.NewSwitch("Enable notifications", false, nil),
		ui.NewSlider("Dialog intensity", 0, 100, 5, 50, nil),
		ui.NewSelect(
			"Dialog preset",
			[]ui.Option{{Key: "one", Label: "First preset"}, {Key: "two", Label: "Second preset"}, {Key: "three", Label: "Third preset"}},
			"one",
			nil,
		),
		closeButton,
	)
	dialog.WithStyle(ui.Style{Width: 360, Padding: 20, Gap: 14, Fill: theme.Surface, Radius: 12, Scroll: true})
	dialog.SetHidden(true)

	motion, bindMotion := motionShowcase(update, check)
	performance, bindPerformance := performanceShowcase(update, check)
	tabs := ui.NewTabs("Playground sections", []ui.Tab{
		{Key: "controls", Label: "Controls", Content: controlShowcase(update, check)},
		{Key: "feedback", Label: "Feedback", Content: feedbackShowcase(update, check)},
		{Key: "layout", Label: "Layout", Content: formShowcase(update, check)},
		{Key: "game", Label: "Game UI", Content: gameShowcase(art.game, update, check)},
		{Key: "charts", Label: "Charts", Content: chartShowcase(update, check)},
		{Key: "motion", Label: "Motion", Content: motion},
		{Key: "panels", Label: "Panels", Content: panelShowcase(art.panels, update, check)},
		{Key: "performance", Label: "Performance", Content: performance},
	}, "controls", func(change ui.Change) { update("Section: " + change.Text) })
	opener := ui.NewButton("Test modal focus", func() { check(view.PushModal(dialog)) })
	opener.WithTooltip("Opens a GPU-rendered modal. Tab stays inside; Escape restores focus here.")
	content := ui.Column(
		ui.NewText("ASHURA   /   UI PLAYGROUND").WithStyle(ui.Style{FontSize: 13, Foreground: theme.Accent}),
		ui.NewText("A small toolkit. A complete interface.").WithStyle(ui.Style{FontSize: 28}),
		ui.NewText("Flow layouts, clear actions and reusable controls. No DOM widgets — every pixel is drawn with WebGPU.").
			WithStyle(ui.Style{Foreground: theme.Muted, FontSize: 15}),
		tabs,
		opener,
		status,
		ui.NewText("Tab navigates · Arrow keys switch tabs and adjust values · Enter accepts · Escape dismisses · Wheel or drag the scrollbar to scroll.").
			WithStyle(ui.Style{FontSize: 13, Foreground: theme.Muted}),
	).WithStyle(ui.Style{MaxWidth: 800, Gap: 15})

	return ui.Column(content, dialog).
			WithStyle(ui.Style{Padding: 24, Fill: theme.Background, Scroll: true}),
		func(attached *ui.View) {
			view = attached
			bindMotion(view)
			bindPerformance(view)
		}
}

// Status messages can include a maximum-length input plus a descriptive prefix.
// Keep that presentation within the UI text bound without rejecting valid input.
func statusMessage(text string) string {
	if len(text) <= ui.MaxTextBytes {
		return text
	}

	const suffix = "…"

	end := ui.MaxTextBytes - len(suffix)
	for !utf8.RuneStart(text[end]) {
		end--
	}

	return text[:end] + suffix
}

func card(children ...*ui.Node) *ui.Node {
	return ui.Column(children...).WithStyle(ui.Style{Padding: 20, Gap: 14, Fill: ui.DarkTheme().Surface, Radius: 12})
}

func action(label string, variant ui.ButtonVariant, activate func()) *ui.Node {
	button := ui.NewButton(label, activate).WithStyle(ui.Style{Basis: 130, Grow: 1})
	button.WithButtonVariant(variant)

	return button
}
