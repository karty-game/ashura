//nolint:mnd // The stress sample declares row counts, dimensions and deterministic sample values explicitly.
package main

import (
	"fmt"

	"github.com/karty-game/karty-webgpu/pkg/ui"
)

type performanceDemo struct {
	check      func(error)
	view       *ui.View
	list       *ui.Node
	snapshot   *ui.Node
	selection  *ui.Node
	count      int
	generation int
	created    int
	updates    map[int]int
	rows       map[*ui.Node]*performanceRow
	update     func(string)
}

type performanceRow struct{ index int }

func performanceShowcase(update func(string), check func(error)) (*ui.Node, func(*ui.View)) {
	demo := &performanceDemo{
		check: check, count: 10000, updates: make(map[int]int), rows: make(map[*ui.Node]*performanceRow), update: update,
		snapshot:  ui.NewText("Scroll the list, then inspect its retained work."),
		selection: ui.NewText("Choose a row to inspect its current logical identity."),
	}
	demo.list = ui.NewVirtualList(ui.VirtualListOptions{
		Count: demo.count, RowHeight: 44, Overscan: 2, Create: demo.createRow, Bind: demo.bindRow,
	}).WithStyle(ui.Style{Height: 320, Padding: 8, Fill: ui.DarkTheme().Background, Radius: 8})
	demo.list.WithLabel("Expedition results", "A recycled list of expedition scores. Scroll to load another range.")
	demo.snapshot.WithStyle(ui.Style{FontSize: 13, Foreground: ui.DarkTheme().Muted})
	demo.selection.WithStyle(ui.Style{FontSize: 13, Foreground: ui.DarkTheme().Accent})

	root := ui.Column(
		card(
			ui.NewText("A long list. A small footprint.").WithStyle(ui.Style{FontSize: 21}),
			ui.NewText(
				"Ten thousand results share a small pool of row controls. Scroll, jump or update data without building a widget for every result.",
			),
			demo.countControls(),
			demo.list,
			demo.selection,
			demo.positionControls(),
		),
		card(
			ui.NewText("Change the data, keep the interface").WithStyle(ui.Style{FontSize: 18}),
			ui.NewText(
				"Visible updates refresh the affected rows. Offscreen updates wait until those rows are shown; a new dataset refreshes only the mounted range.",
			),
			demo.updateControls(),
			ui.NewSeparator(),
			demo.snapshot,
			action("Inspect retained work", ui.SecondaryButton, demo.inspect),
			ui.NewText("Counters are snapshots taken on request, so idle frames do no extra sample work. Record slots include the whole playground; they are not GPU draw calls.").
				WithStyle(ui.Style{FontSize: 13, Foreground: ui.DarkTheme().Muted}),
		),
	).WithStyle(ui.Style{Gap: 16})

	return root, func(view *ui.View) { demo.view = view }
}

func (demo *performanceDemo) createRow() *ui.Node {
	row := &performanceRow{index: -1}
	button := ui.NewButton("", func() {
		demo.check(demo.selection.SetText(fmt.Sprintf("Selected expedition %05d · dataset %d", row.index+1, demo.generation+1)))
		demo.update(fmt.Sprintf("Selected logical row %d; the recycled control uses its current bound identity.", row.index+1))
	}).WithStyle(ui.Style{Height: 44, FontSize: 14})
	button.WithButtonVariant(ui.QuietButton)

	demo.rows[button] = row
	demo.created++

	return button
}

func (demo *performanceDemo) bindRow(node *ui.Node, index int) error {
	demo.rows[node].index = index
	revision := demo.generation + demo.updates[index]
	score := (index*137 + revision*71 + 375) % 10000

	return node.SetText(fmt.Sprintf("Expedition %05d   ·   Score %04d   ·   v%d", index+1, score, revision+1))
}

func (demo *performanceDemo) countControls() *ui.Node {
	return ui.Row(
		action("10,000 rows", ui.PrimaryButton, func() { demo.setCount(10000) }),
		action("100,000 rows", ui.SecondaryButton, func() { demo.setCount(100000) }),
	).WithStyle(ui.Style{Wrap: true, Gap: 12})
}

func (demo *performanceDemo) setCount(count int) {
	if err := demo.list.SetVirtualCount(count); err != nil {
		demo.check(err)

		return
	}

	demo.count = count
	demo.update(fmt.Sprintf("The list now represents %d results using the same viewport-sized row pool.", count))
}

func (demo *performanceDemo) positionControls() *ui.Node {
	return ui.Row(
		action("First result", ui.SecondaryButton, func() { demo.jump(0) }),
		action("Middle result", ui.SecondaryButton, func() { demo.jump(demo.count / 2) }),
		action("Last result", ui.SecondaryButton, func() { demo.jump(demo.count - 1) }),
	).WithStyle(ui.Style{Wrap: true, Gap: 12})
}

func (demo *performanceDemo) jump(index int) {
	if err := demo.list.ScrollToRow(index); err != nil {
		demo.check(err)

		return
	}

	demo.update(fmt.Sprintf("Jumped directly to result %d without walking intermediate rows.", index+1))
}

func (demo *performanceDemo) updateControls() *ui.Node {
	return ui.Row(
		action("Update visible row", ui.PrimaryButton, demo.updateVisible),
		action("Update offscreen row", ui.SecondaryButton, demo.updateOffscreen),
		action("New dataset", ui.SecondaryButton, demo.updateAll),
	).WithStyle(ui.Style{Wrap: true, Gap: 12})
}

func (demo *performanceDemo) updateVisible() {
	index := min(demo.count-1, max(0, int(demo.list.ScrollOffset()/44)))
	demo.updates[index]++
	demo.check(demo.list.RefreshVirtualRows(index, 1))

	demo.update(fmt.Sprintf("Result %d updated. Other row data is unchanged.", index+1))
}

func (demo *performanceDemo) updateOffscreen() {
	index := demo.count - 1
	if demo.list.VirtualListStats().End == demo.count {
		index = 0
	}

	demo.updates[index]++
	demo.check(demo.list.RefreshVirtualRows(index, 1))

	demo.update(fmt.Sprintf("Offscreen result %d updated. Its control will refresh when scrolled into view.", index+1))
}

func (demo *performanceDemo) updateAll() {
	demo.generation++
	demo.check(demo.list.RefreshVirtualRows(0, demo.count))

	demo.update(fmt.Sprintf("Dataset %d replaces all %d scores; only the mounted rows need binding.", demo.generation+1, demo.count))
}

func (demo *performanceDemo) inspect() {
	if demo.view == nil {
		return
	}

	list, view := demo.list.VirtualListStats(), demo.view.Stats()
	demo.check(demo.snapshot.SetText(fmt.Sprintf(
		"%d logical rows · %d mounted · %d controls created · %d row bindings\n%d playground nodes · %d retained record slots",
		list.Count, list.Mounted, demo.created, list.Bindings, view.Nodes, view.Records,
	)))

	demo.update("Retained-work snapshot refreshed. The counters remain still until requested again.")
}
