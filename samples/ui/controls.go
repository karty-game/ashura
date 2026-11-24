//nolint:mnd // Sample control bounds, preferred widths and spacing are explicit.
package main

import (
	"fmt"

	"github.com/karty-game/ashura/pkg/ui"
)

func controlShowcase(update func(string), check func(error)) *ui.Node {
	name := ui.NewInput("Display name", "Explorer", "Type your name", func(change ui.Change) { update("Name: " + change.Text) })
	name.OnCommit(func(change ui.Change) { update("Name submitted: " + change.Text) })
	quality := ui.NewSelect("Quality preset", []ui.Option{
		{Key: "low", Label: "Low — save power"}, {Key: "balanced", Label: "Balanced"}, {Key: "high", Label: "High quality"},
		{Key: "ultra", Label: "Ultra quality"}, {Key: "cinematic", Label: "Cinematic"}, {Key: "custom", Label: "Custom"},
		{Key: "unavailable", Label: "Unavailable preset", Disabled: true}, {Key: "experimental", Label: "Experimental"},
	}, "balanced", func(change ui.Change) { update("Quality preset: " + change.Text) })
	volumeLabel := ui.NewText("Volume · 65%").WithStyle(ui.Style{FontSize: 14})
	volume := ui.NewSlider("Volume", 0, 100, 1, 65, func(change ui.Change) {
		check(volumeLabel.SetText(fmt.Sprintf("Volume · %.0f%%", change.Value)))
		update(fmt.Sprintf("Volume changed: %.0f%%", change.Value))
	})
	volume.OnCommit(func(change ui.Change) { update(fmt.Sprintf("Volume committed: %.0f%%", change.Value)) })

	adjust := func(delta float64) {
		check(volume.SetValue(volume.Value() + delta))
		check(volumeLabel.SetText(fmt.Sprintf("Volume · %.0f%%", volume.Value())))
	}
	minus := ui.NewButton("−", func() { adjust(-5) }).WithStyle(ui.Style{Width: 48})
	plus := ui.NewButton("+", func() { adjust(5) }).WithStyle(ui.Style{Width: 48})
	volume.WithStyle(ui.Style{Grow: 1})

	hints := ui.NewCheckbox(
		"Show performance hints",
		true,
		func(change ui.Change) { update(fmt.Sprintf("Performance hints: %t", change.Checked)) },
	)
	sound := ui.NewSwitch("Sound enabled", true, func(change ui.Change) { update(fmt.Sprintf("Sound enabled: %t", change.Checked)) })
	pin := ui.NewToggle("Pin panel", false, func(change ui.Change) { update(fmt.Sprintf("Panel pinned: %t", change.Checked)) }).
		WithStyle(ui.Style{Basis: 130, Grow: 1})
	clicks := 0
	apply := action(
		"Apply",
		ui.PrimaryButton,
		func() { clicks++; update(fmt.Sprintf("Applied for %s · %d activations", name.Text(), clicks)) },
	)
	apply.WithTooltip("Apply the current settings. Primary actions use the accent color; secondary actions use blue.")

	reset := action("Reset", ui.SecondaryButton, func() {
		check(name.SetText("Explorer"))
		check(quality.SetSelected("balanced"))

		hints.SetChecked(true)
		sound.SetChecked(true)
		pin.SetChecked(false)

		check(volume.SetValue(65))
		check(volumeLabel.SetText("Volume · 65%"))

		update("Settings reset.")
	})
	disabled := action("Disabled", ui.SecondaryButton, func() { update("Disabled action must not run.") })
	disabled.SetDisabled(true)

	help := ui.NewButton("Help", func() { update("Hover Help or focus it with Tab to open its tooltip.") }).WithStyle(ui.Style{Width: 84})
	help.WithTooltip(
		"Controls share one interaction model for mouse, touch and keyboard. Escape dismisses this help without moving focus.",
	)

	heading := ui.NewText("Make it yours").WithStyle(ui.Style{FontSize: 21, Grow: 1})
	settings := card(
		ui.Row(heading, help).WithStyle(ui.Style{Gap: 12}),
		ui.NewForm(ui.NewField("Display name", name), ui.NewField("Quality preset", quality),
			ui.Column(volumeLabel, ui.Row(minus, volume, plus).WithStyle(ui.Style{Gap: 10})), hints, sound),
		ui.NewSeparator(),
		ui.Row(apply, reset, pin, disabled).WithStyle(ui.Style{Wrap: true, Gap: 10}),
	)

	return ui.Column(settings, collectionShowcase(update, check)).WithStyle(ui.Style{Gap: 16})
}

func collectionShowcase(update func(string), check func(error)) *ui.Node {
	items := make([]*ui.Node, 0, 12)

	for index := range 12 {
		var button *ui.Node

		button = ui.NewButton(fmt.Sprintf("Collection item %02d", index+1), func() { update("Selected " + button.Text()) })
		button.WithButtonVariant(ui.QuietButton)
		items = append(items, button)
	}

	scroller := ui.Column(items...).
		WithStyle(ui.Style{Height: 190, Padding: 12, Gap: 8, Fill: ui.DarkTheme().Background, Radius: 8, Scroll: true})
	pages := ui.NewPagination("Collection pages", 1, 8, func(change ui.Change) {
		page := int(change.Value)
		for index, item := range items {
			check(item.SetText(fmt.Sprintf("Collection item %02d", (page-1)*12+index+1)))
		}

		update(fmt.Sprintf("Collection page %d of 8", page))
	})

	return card(ui.NewText("A scrollable panel").WithStyle(ui.Style{FontSize: 18}), scroller, pages)
}
