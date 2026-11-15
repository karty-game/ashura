//nolint:mnd // Sample progress steps, preferred form widths and text metrics are explicit.
package main

import (
	"fmt"
	"strings"

	"github.com/karty-game/karty-webgpu/pkg/ui"
)

func feedbackShowcase(update func(string), check func(error)) *ui.Node {
	progress := ui.NewProgress("Build progress", 0.65)
	label := ui.NewText("Build progress · 65%")
	advance := action("Advance", ui.PrimaryButton, func() {
		value := progress.Progress() + 0.1
		if value > 1 {
			value = 0
		}

		check(progress.SetProgress(value))
		check(label.SetText(fmt.Sprintf("Build progress · %.0f%%", value*100)))
		update(fmt.Sprintf("Build progress: %.0f%%", value*100))
	})

	return ui.Column(
		card(ui.NewText("Feedback with a purpose").WithStyle(ui.Style{FontSize: 21}), label, progress, advance),
		ui.NewSeparator(),
		ui.NewAlert(
			ui.InfoAlert,
			"Prepared ahead of time",
			"Text uses a prepared MSDF atlas. Dynamic strings are shaped when they change.",
		),
		ui.NewAlert(ui.SuccessAlert, "Changes saved", "The current settings are ready to use."),
		ui.NewAlert(ui.WarningAlert, "Check before continuing", "Warnings remain visible and do not steal keyboard focus."),
		ui.NewAlert(ui.DangerAlert, "Connection unavailable", "Retry the operation or keep working with the current data."),
	).WithStyle(ui.Style{Gap: 16})
}

func formShowcase(update func(string), check func(error)) *ui.Node {
	name := ui.NewInput("Full name", "Ada Lovelace", "Your name", nil)
	email := ui.NewInput("Email address", "ada@", "name@example.com", nil)
	nameField, emailField := ui.NewField("Full name", name), ui.NewField("Email address", email)
	nameField.WithHelpText("Used in your public profile.")
	emailField.WithHelpText("Validation messages take their own space in the flow.")

	project := ui.NewField("Project name", ui.NewInput("Project name", "My next project", "A descriptive name", nil))
	project.WithHelpText("No absolute coordinates: labels, inputs and messages determine each field's height.")

	validate := action("Validate form", ui.PrimaryButton, func() {
		nameInvalid := strings.TrimSpace(name.Text()) == ""
		emailInvalid := !strings.Contains(email.Text(), "@") || !strings.Contains(email.Text(), ".")

		if nameInvalid {
			check(nameField.SetErrorText("Enter your name."))
		} else {
			check(nameField.SetErrorText(""))
		}

		if emailInvalid {
			check(emailField.SetErrorText("Enter an address such as name@example.com."))
		} else {
			check(emailField.SetErrorText(""))
		}

		if nameInvalid || emailInvalid {
			update("Form needs attention. The error reflows the layout.")
		} else {
			update("Form is ready.")
		}
	})
	clearButton := action("Clear errors", ui.SecondaryButton, func() {
		check(nameField.SetErrorText(""))
		check(emailField.SetErrorText(""))

		update("Validation messages cleared.")
	})

	return card(
		ui.NewText("Design relationships, not positions").WithStyle(ui.Style{FontSize: 21}),
		ui.NewText(
			"Resize the window: these fields share a row when space permits, then stack. Help and error messages wrap and push later fields down.",
		),
		ui.NewForm(ui.Row(nameField, emailField).WithStyle(ui.Style{Wrap: true, Gap: 16}), project,
			ui.Row(validate, clearButton).WithStyle(ui.Style{Wrap: true, Gap: 12})),
	)
}
