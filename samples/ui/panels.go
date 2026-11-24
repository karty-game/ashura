//nolint:mnd // Panel source cuts, logical border scales and demo dimensions are explicit.
package main

import (
	"fmt"

	"github.com/karty-game/ashura/pkg/ui"
)

func panelShowcase(art panelArt, update func(string), check func(error)) *ui.Node {
	return ui.Column(
		card(
			ui.NewText("Panels with character").WithStyle(ui.Style{FontSize: 21}),
			ui.NewText("A landscape behind your content, a decorative frame around it. Both come from independent PNG assets."),
		),
		panelFrameShowcase(art, update, check),
		panelDecorations(art, update),
		panelImageFits(art),
		panelScrollShowcase(art, update),
	).WithStyle(ui.Style{Gap: 16})
}

func panelSlice(borderOnly bool, scale float32) *ui.NineSlice {
	return &ui.NineSlice{Left: 32, Top: 32, Right: 32, Bottom: 32, Scale: scale, BorderOnly: borderOnly}
}

type panelFrameDemo struct {
	check                                     func(error)
	art                                       panelArt
	caption, journal, illustration            *ui.Node
	steel, compact, showBackground, showFrame bool
}

func panelFrameShowcase(art panelArt, update func(string), check func(error)) *ui.Node {
	caption := ui.NewText("Golden frame · Wide").WithStyle(ui.Style{FontSize: 13, Foreground: ui.DarkTheme().Muted})
	name := ui.NewInput("Expedition name", "Aurora Trail", "Name this expedition", nil)
	illustration := ui.Column(
		ui.NewText("Field journal").WithStyle(ui.Style{FontSize: 23}),
		ui.NewText("A view of the mountains, room for the adventure."),
		ui.NewField("Expedition name", name),
	).WithStyle(ui.Style{Grow: 1, Padding: 12, Gap: 13, Radius: 4})
	journal := ui.Column(illustration).WithStyle(ui.Style{Height: 230, Padding: 16, Fill: ui.DarkTheme().Surface})
	demo := &panelFrameDemo{
		art: art, caption: caption, journal: journal, illustration: illustration, check: check,
		showBackground: true, showFrame: true,
	}

	journal.WithPanelBackground(ui.PanelBackground{Image: art.gold, Slice: panelSlice(true, 0.5)})
	illustration.WithPanelBackground(ui.PanelBackground{Image: art.landscape, Fit: ui.CoverImage, Tint: ui.Color{0.72, 0.78, 0.9, 1}})

	switchFrame := motionAction("Switch frame", ui.SecondaryButton, func() {
		demo.steel = !demo.steel

		demo.refresh()
		update("Panel frame changed; the expedition controls keep their state.")
	})
	resize := motionAction("Resize panel", ui.SecondaryButton, func() {
		demo.compact = !demo.compact

		demo.refresh()
		update("The panel changed size while its decorative corners stayed the same size.")
	})
	show := ui.NewCheckbox("Show background", true, func(change ui.Change) {
		demo.showBackground = change.Checked

		demo.refresh()
		update("Panel background visibility changed.")
	})
	showFrame := ui.NewCheckbox("Show frame", true, func(change ui.Change) {
		demo.showFrame = change.Checked

		demo.refresh()
		update("Panel frame visibility changed.")
	})

	return card(
		ui.NewText("One frame, many sizes").WithStyle(ui.Style{FontSize: 18}),
		caption,
		journal,
		ui.Row(switchFrame, resize).WithStyle(ui.Style{Wrap: true, Gap: 12}),
		ui.Row(show, showFrame).WithStyle(ui.Style{Wrap: true, Gap: 12}),
	)
}

func (demo *panelFrameDemo) refresh() {
	image, label := demo.art.gold, "Golden frame"
	if demo.steel {
		image, label = demo.art.steel, "Steel frame"
	}

	style := demo.journal.Style()

	style.Width, style.Height = 0, 230
	if demo.compact {
		style.Width, style.Height = 320, 290
		label += " · Compact"
	} else {
		label += " · Wide"
	}

	demo.check(demo.journal.SetStyle(style))

	background := ui.PanelBackground{}
	picture := ui.PanelBackground{}

	if demo.showFrame {
		background = ui.PanelBackground{Image: image, Slice: panelSlice(true, 0.5)}
	} else {
		label += " · Frame hidden"
	}

	if demo.showBackground {
		picture = ui.PanelBackground{Image: demo.art.landscape, Fit: ui.CoverImage, Tint: ui.Color{0.72, 0.78, 0.9, 1}}
	} else {
		label += " · Background hidden"
	}

	demo.check(demo.journal.SetPanelBackground(background))
	demo.check(demo.illustration.SetPanelBackground(picture))
	demo.check(demo.caption.SetText(label))
}

func panelDecorations(art panelArt, update func(string)) *ui.Node {
	progress := ui.NewProgress("Waypoints discovered", 0.72)
	journey := ui.Column(
		ui.NewText("Ready for the next quest").WithStyle(ui.Style{FontSize: 20}),
		ui.NewText("Waypoints discovered · 72%"),
		progress,
		motionAction("Continue journey", ui.PrimaryButton, func() { update("Your next expedition is ready.") }),
	).WithStyle(ui.Style{Padding: 30, Gap: 10, Fill: ui.Hex(0x153335)})
	journey.WithPanelBackground(ui.PanelBackground{
		Image: art.gold, Slice: panelSlice(true, 0.75), Tint: ui.Color{0.65, 1, 0.92, 1},
	})

	mini := ui.Column().WithStyle(ui.Style{Width: 40, Height: 28})
	mini.WithPanelBackground(ui.PanelBackground{Image: art.steel, Slice: panelSlice(false, 1)})

	return card(
		ui.NewText("Border decoration").WithStyle(ui.Style{FontSize: 18}),
		ui.NewText("A tinted frame leaves the center clear for your own surface color."),
		journey,
		ui.Row(mini, ui.NewText("Small frames compress safely").WithStyle(ui.Style{FontSize: 13})).WithStyle(ui.Style{Gap: 12}),
	)
}

func panelImageFits(art panelArt) *ui.Node {
	cards := make([]*ui.Node, 0, 3)

	for _, example := range []struct {
		label string
		fit   ui.ImageFit
	}{
		{"Stretch", ui.StretchImage},
		{"Contain", ui.ContainImage},
		{"Cover", ui.CoverImage},
	} {
		image := ui.Column().WithStyle(ui.Style{Height: 170, Fill: ui.Hex(0x111b2b), Radius: 12})
		image.WithPanelBackground(ui.PanelBackground{Image: art.landscape, Fit: example.fit})
		cards = append(cards, ui.Column(
			ui.NewText(example.label).WithStyle(ui.Style{FontSize: 14}), image,
		).WithStyle(ui.Style{Basis: 220, Grow: 1, Gap: 10}))
	}

	return card(
		ui.NewText("Image backgrounds").WithStyle(ui.Style{FontSize: 18}),
		ui.NewText("Use the whole image, preserve its proportions or fill the panel with a centered crop."),
		ui.Row(cards...).WithStyle(ui.Style{Wrap: true, Gap: 16}),
	)
}

func panelScrollShowcase(art panelArt, update func(string)) *ui.Node {
	entries := make([]*ui.Node, 0, 8)

	for index := range 8 {
		label := fmt.Sprintf("Dispatch %02d", index+1)
		entries = append(entries, ui.NewButton(label, func() { update(label + " opened.") }).WithStyle(ui.Style{FontSize: 14}))
	}

	scroller := ui.Column(entries...).WithStyle(ui.Style{Height: 180, Padding: 22, Gap: 10, Scroll: true, Fill: ui.Hex(0x182b36)})
	scroller.WithPanelBackground(ui.PanelBackground{Image: art.steel, Slice: panelSlice(true, 0.5)})

	return card(
		ui.NewText("A fixed frame, scrolling contents").WithStyle(ui.Style{FontSize: 18}),
		ui.NewText("Scroll the dispatches. The frame stays attached to the panel."),
		scroller,
	)
}
