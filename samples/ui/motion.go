//nolint:mnd // Sample button heights, preferred widths and validation length are explicit.
package main

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/karty-game/ashura/pkg/ui"
)

func motionShowcase(update func(string), check func(error)) (*ui.Node, func(*ui.View)) {
	var view *ui.View

	effects := ui.NewCheckbox("Interaction effects", true, func(change ui.Change) {
		if view == nil {
			return
		}

		theme := view.Theme()
		theme.Motion.Disabled = !change.Checked
		check(view.SetTheme(theme))

		if change.Checked {
			update("Interaction effects enabled.")
		} else {
			update("Interaction effects disabled.")
		}
	})
	root := ui.Column(
		card(
			ui.NewText("Small details. Better feedback.").WithStyle(ui.Style{FontSize: 21}),
			ui.NewText("Hover, press and release. Soft transitions help each action feel connected to your input."),
			effects,
			ui.NewText("Your device's reduced-motion preference is always respected.").
				WithStyle(ui.Style{FontSize: 13, Foreground: ui.DarkTheme().Muted}),
		),
		motionButtons(update, check),
		motionFields(update, check),
	).WithStyle(ui.Style{Gap: 16})

	return root, func(attached *ui.View) {
		view = attached
		effects.SetChecked(!view.Theme().Motion.Disabled)
	}
}

func motionButtons(update func(string), check func(error)) *ui.Node {
	confirmations := 0
	count := ui.NewText("Confirmed: 0").WithStyle(ui.Style{FontSize: 14, Foreground: ui.DarkTheme().Muted})
	confirm := motionAction("Confirm action", ui.PrimaryButton, func() {
		confirmations++
		check(count.SetText(fmt.Sprintf("Confirmed: %d", confirmations)))

		update("Action confirmed.")
	})
	secondary := motionAction("Secondary action", ui.SecondaryButton, func() { update("Secondary action accepted.") })
	quiet := motionAction("Quiet action", ui.QuietButton, func() { update("Quiet action accepted.") })
	favorite := ui.NewToggle("Favorite", false, func(change ui.Change) {
		if change.Checked {
			update("Favorite added.")
		} else {
			update("Favorite removed.")
		}
	}).WithStyle(ui.Style{Basis: 160, Grow: 1, Height: 48})
	help := ui.NewButton("", func() { update("Try pressing and holding a button, then releasing.") }).
		WithStyle(ui.Style{Width: 48, Height: 48})
	help.WithLabel("Motion help", "")
	help.WithButtonVisual(ui.ButtonVisual{Icon: "?", Size: 20})
	help.WithTooltip("Try a pointer, touch, Enter or Space. Each action keeps the same focus and activation behavior.")

	disabled := motionAction("Unavailable", ui.SecondaryButton, nil)
	disabled.SetDisabled(true)

	return card(
		ui.NewText("Actions with a little presence").WithStyle(ui.Style{FontSize: 18}),
		ui.NewText("Press anywhere on a button. Move between actions, or use Tab to see keyboard focus."),
		ui.Row(confirm, secondary, quiet).WithStyle(ui.Style{Wrap: true, Gap: 12}),
		ui.Row(favorite, help, disabled).WithStyle(ui.Style{Wrap: true, Gap: 12}),
		count,
	)
}

func motionAction(label string, variant ui.ButtonVariant, activate func()) *ui.Node {
	button := ui.NewButton(label, activate).WithStyle(ui.Style{Basis: 180, Grow: 1, Height: 48})
	button.WithButtonVariant(variant)

	return button
}

func motionFields(update func(string), check func(error)) *ui.Node {
	tag := ui.NewInput("Player tag", "", "Choose a tag", nil)
	tagField := ui.NewField("Player tag", tag)
	tagField.WithHelpText("Use at least 3 characters. Enter also validates your tag.")

	note := ui.NewInput("Session note", "Ready for the next run", "A short note", nil)
	noteField := ui.NewField("Session note", note)
	noteField.WithHelpText("Move focus between fields to follow the active indicator.")

	validate := func() {
		value := strings.TrimSpace(tag.Text())
		if utf8.RuneCountInString(value) < 3 {
			check(tagField.SetErrorText("Use at least 3 characters for your player tag."))

			update("Please review your player tag.")

			return
		}

		check(tagField.SetErrorText(""))

		update("Player tag ready: " + value)
	}

	tag.OnCommit(func(ui.Change) { validate() })

	clearValidation := motionAction("Clear validation", ui.SecondaryButton, func() {
		check(tagField.SetErrorText(""))

		update("Validation cleared.")
	})

	return card(
		ui.NewText("Text fields that respond").WithStyle(ui.Style{FontSize: 18}),
		ui.NewText("Focus follows the active field. Validation keeps the label and help close to your input."),
		ui.NewForm(
			ui.Row(tagField, noteField).WithStyle(ui.Style{Wrap: true, Gap: 16}),
			ui.Row(motionAction("Validate tag", ui.PrimaryButton, validate), clearValidation).WithStyle(ui.Style{Wrap: true, Gap: 12}),
		),
	)
}
