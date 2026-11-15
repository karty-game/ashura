//nolint:mnd // The debug overlay uses explicit bounded text sizes and spacing.
package debug

import "github.com/karty-game/karty-webgpu/pkg/ui"

const (
	labelDrawCalls = "Draw calls"
	labelTriangles = "Triangles"
)

func (widgets *panelUI) buildRows(theme ui.Theme) *ui.Node {
	groups := []struct {
		title  string
		labels []string
	}{
		{"Frame · CPU wall time", []string{"FPS", "Frame interval", "Engine CPU"}},
		{"Scene 3D · last frame", []string{labelDrawCalls, "Mesh instances", labelTriangles}},
		{"2D sprites · last frame", []string{labelDrawCalls, "Sprites", labelTriangles}},
		{"UI · includes debug", []string{labelDrawCalls, "Quads", labelTriangles}},
		{"Total · last frame", []string{labelDrawCalls, "Compute dispatches"}},
		{"CPU phases", []string{
			"Update / controls", "Compute schedule", "Scene snapshot", "Geometry / uploads", "Surface acquisition",
			"Compute encoding", "Draw / material prep", "Submit / present", "GPU elapsed", "Panel refresh",
		}},
		{
			"Memory · sampled",
			[]string{
				"Go heap in use ≈", "Go live after GC", "Go allocated / frame ≈", "Go objects / frame ≈", "Go allocated / second ≈",
				"JS live heap ≈", "JS net heap Δ / second", "JS allocated / frame",
			},
		},
		{"Textures", []string{"Loaded App handles", "GPU resident", "Resident payload ≈"}},
	}

	children := make([]*ui.Node, 0, rowCount+len(groups)+1)
	row := 0

	for _, group := range groups {
		children = append(children, ui.NewText(group.title).WithStyle(ui.Style{FontSize: 13, Foreground: theme.Accent}))
		for _, label := range group.labels {
			widgets.rows[row] = ui.NewText("—").WithStyle(ui.Style{Grow: 1, FontSize: 12})
			widgets.text[row] = "—"
			children = append(children, ui.Row(
				ui.NewText(label).WithStyle(ui.Style{Width: 176, FontSize: 12, Foreground: theme.Muted}),
				widgets.rows[row],
			).WithStyle(ui.Style{Gap: 6}))
			row++
		}
	}

	children = append(children, ui.NewText(
		"Rendering categories separate scene meshes, 2D sprites and UI, including this panel. "+
			"CPU timings include uploads and waits. GPU elapsed and JS allocation counters are unavailable. "+
			"Resident payload excludes UI/sprite pages, targets and driver overhead. Sampling runs at most 4×/s; "+
			"allocation rates include the panel itself. Go allocation counters can lag runtime flushes; "+
			"a zero sample does not prove zero allocations.",
	).WithStyle(ui.Style{FontSize: 11, Foreground: theme.Muted}))

	return ui.Column(children...).WithLabel("Runtime statistics", "Scroll to inspect frame, memory and texture counters").
		WithStyle(ui.Style{Grow: 1, Scroll: true, Gap: 6})
}
