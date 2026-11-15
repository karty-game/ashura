//nolint:mnd // Sample icon dimensions, prices and inventory bounds are explicit.
package main

import (
	"fmt"

	"github.com/karty-game/karty-webgpu/pkg/ui"
)

func gameShowcase(art gameArt, update func(string), check func(error)) *ui.Node {
	return ui.Column(
		card(
			ui.NewText("Buttons built for games").WithStyle(ui.Style{FontSize: 21}),
			ui.NewText("Compact commands, resource badges and inventory slots share one button model."),
			gameCommands(update),
			ui.NewSeparator(),
			gameCharges(update, check),
		),
		gameBadgeActions(art, update),
		gameInventory(art, update, check),
	).WithStyle(ui.Style{Gap: 16})
}

func gameCommands(update func(string)) *ui.Node {
	equip := action("Equip", ui.PrimaryButton, func() { update("Equipment ready. Leading icons use the button foreground.") })
	equip.WithButtonVisual(ui.ButtonVisual{Icon: "+", Size: 20})
	equip.WithTooltip("Equip the selected item. The icon and label share one focus stop and one activation target.")

	continueButton := action(
		"Continue",
		ui.SecondaryButton,
		func() { update("Continue activated. Trailing icons keep the action readable.") },
	)
	continueButton.WithButtonVisual(ui.ButtonVisual{Icon: "›", Side: ui.TrailingContent, Size: 24})

	return ui.Row(equip, continueButton).WithStyle(ui.Style{Wrap: true, Gap: 12})
}

func gameCharges(update func(string), check func(error)) *ui.Node {
	charges := 3
	label := ui.NewText("Charges: 3").WithStyle(ui.Style{Width: 114, Height: 42, Padding: 10})

	var minus, plus *ui.Node

	refresh := func() {
		check(label.SetText(fmt.Sprintf("Charges: %d", charges)))
		minus.SetDisabled(charges == 0)
		plus.SetDisabled(charges == 9)
		update(fmt.Sprintf("Potion charges: %d", charges))
	}
	minus = gameIconButton("−", "Remove a charge", "Remove one potion charge.", func() {
		charges--

		refresh()
	})
	plus = gameIconButton("+", "Add a charge", "Add one potion charge.", func() {
		charges++

		refresh()
	})
	help := gameIconButton("?", "Charge help", "Charges are limited to nine. Empty actions disable at their boundary.", func() {
		update("Charge controls have accessible names even when only a symbol is visible.")
	})

	return ui.Row(label, minus, plus, help).WithStyle(ui.Style{Wrap: true, Gap: 8})
}

func gameIconButton(icon, name, tooltip string, activate func()) *ui.Node {
	button := ui.NewButton("", activate).WithStyle(ui.Style{Width: 44, Height: 42})
	button.WithLabel(name, "")
	button.WithButtonVisual(ui.ButtonVisual{Icon: icon, Size: 20})
	button.WithTooltip(tooltip)

	return button
}

func gameBadgeActions(art gameArt, update func(string)) *ui.Node {
	buy := action("Buy · 250", ui.PrimaryButton, func() { update("Health potion purchased for 250 gold.") })
	buy.WithStyle(ui.Style{Basis: 220, Grow: 1, Height: 54})
	buy.WithButtonVisual(ui.ButtonVisual{Image: art.coin, Size: 30})
	buy.WithTooltip("A leading image badge uses the prepared coin tile from the shared atlas.")

	craft := action("Craft · 12", ui.SecondaryButton, func() { update("Shield crafted using 12 crystals.") })
	craft.WithStyle(ui.Style{Basis: 220, Grow: 1, Height: 54})
	craft.WithButtonVisual(ui.ButtonVisual{Image: art.gem, Side: ui.TrailingContent, Size: 30})
	craft.WithTooltip("A trailing image badge keeps its original colors and transparent edges.")

	return card(
		ui.NewText("Resource badges").WithStyle(ui.Style{FontSize: 18}),
		ui.NewText("Prepared images can sit before or after the label. One atlas is shared by every instance."),
		ui.Row(buy, craft).WithStyle(ui.Style{Wrap: true, Gap: 12}),
	)
}

func gameInventory(art gameArt, update func(string), check func(error)) *ui.Node {
	selection := ui.NewText("Health potion selected · Restores 40 health.").WithStyle(ui.Style{FontSize: 14})
	slots := gameInventorySlots(art, selection, update, check)
	tabs := ui.NewTabs("Inventory categories", []ui.Tab{
		{
			Key: "items", Label: "Items", Visual: ui.ButtonVisual{Image: art.potion, Size: 20},
			Content: ui.Column(slots, selection).WithStyle(ui.Style{Gap: 14}),
		},
		{
			Key: "gear", Label: "Gear", Visual: ui.ButtonVisual{Image: art.shield, Size: 20},
			Content: ui.NewText("Shield equipped · +12 armor. Image tabs keep the same arrow-key navigation."),
		},
		{
			Key: "help", AccessibleLabel: "Inventory help", Visual: ui.ButtonVisual{Icon: "?", Size: 20},
			Content: ui.NewText("Choose an item slot with a pointer or Tab and Space. Arrow keys switch the category tabs."),
		},
	}, "items", func(change ui.Change) { update("Inventory category: " + change.Text) })

	return card(
		ui.NewText("An inventory, without extra controls").WithStyle(ui.Style{FontSize: 18}),
		ui.NewText("Image-only slots are ordinary toggle buttons. Icons and image tabs keep their names, focus and selected state."),
		tabs,
	)
}

func gameInventorySlots(art gameArt, selection *ui.Node, update func(string), check func(error)) *ui.Node {
	items := []struct {
		name, detail string
		image        *ui.ImageAsset
	}{
		{"Health potion", "Restores 40 health.", art.potion},
		{"Guardian shield", "+12 armor.", art.shield},
		{"Sea crystal", "A rare crafting material.", art.gem},
		{"Gold token", "Trade for supplies.", art.coin},
	}

	buttons := make([]*ui.Node, len(items))
	for index, item := range items {
		button := ui.NewToggle("", index == 0, func(ui.Change) {
			for current, sibling := range buttons {
				sibling.SetChecked(current == index)
			}

			check(selection.SetText(item.name + " selected · " + item.detail))
			update("Selected " + item.name)
		}).WithStyle(ui.Style{Width: 64, Height: 68, Radius: 10})
		button.WithButtonVisual(ui.ButtonVisual{Image: item.image, Size: 42})
		button.WithLabel(item.name, item.detail)
		button.WithTooltip(item.name + " · " + item.detail)
		buttons[index] = button
	}

	return ui.Row(buttons...).WithStyle(ui.Style{Wrap: true, Gap: 8})
}
