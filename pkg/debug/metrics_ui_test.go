package debug

import (
	"testing"

	"github.com/karty-game/ashura/pkg/engine"
	"github.com/karty-game/ashura/pkg/ui"
	"github.com/karty-game/ashura/pkg/ui/fonts/defaultfont"
)

func TestRenderingRowsSeparateSceneSpritesAndDebugUI(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		frame    engine.FrameStats
		expected map[string]string
	}{
		{
			name: "four cubes floor and debug panel",
			frame: engine.FrameStats{
				DrawCalls: 4, Instances: 397, Triangles: 834,
				Scene: engine.DrawStats{DrawCalls: 3, Instances: 5, Triangles: 50},
				UI:    engine.DrawStats{DrawCalls: 1, Instances: 392, Triangles: 784},
			},
			expected: map[string]string{
				"Scene 3D · last frame/Mesh instances": "5",
				"Scene 3D · last frame/Triangles":      "50",
				"2D sprites · last frame/Sprites":      "0",
				"UI · includes debug/Quads":            "392",
				"UI · includes debug/Triangles":        "784",
				"Total · last frame/Draw calls":        "4",
			},
		},
		{
			name: "all three rendering categories",
			frame: engine.FrameStats{
				DrawCalls: 9, Instances: 85, Triangles: 255, ComputeDispatches: 6,
				Scene:   engine.DrawStats{DrawCalls: 2, Instances: 7, Triangles: 99},
				Sprites: engine.DrawStats{DrawCalls: 3, Instances: 31, Triangles: 62},
				UI:      engine.DrawStats{DrawCalls: 4, Instances: 47, Triangles: 94},
			},
			expected: map[string]string{
				"Scene 3D · last frame/Draw calls":      "2",
				"Scene 3D · last frame/Mesh instances":  "7",
				"Scene 3D · last frame/Triangles":       "99",
				"2D sprites · last frame/Draw calls":    "3",
				"2D sprites · last frame/Sprites":       "31",
				"2D sprites · last frame/Triangles":     "62",
				"UI · includes debug/Draw calls":        "4",
				"UI · includes debug/Quads":             "47",
				"UI · includes debug/Triangles":         "94",
				"Total · last frame/Draw calls":         "9",
				"Total · last frame/Compute dispatches": "6",
			},
		},
	}

	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			widgets, rows := newRenderingRows(t)
			widgets.render(Report{Frame: test.frame})

			if widgets.failure != nil {
				t.Fatal(widgets.failure)
			}

			actual := renderingRows(rows)
			for key, want := range test.expected {
				if actual[key] != want {
					t.Errorf("%s: got %q, want %q", key, actual[key], want)
				}
			}
		})
	}
}

func newRenderingRows(t *testing.T) (*panelUI, *ui.Node) {
	t.Helper()

	font, err := defaultfont.Load()
	if err != nil {
		t.Fatal(err)
	}

	view, err := ui.New(ui.Column(), font)
	if err != nil {
		t.Fatal(err)
	}

	widgets := &panelUI{view: view}

	rows := widgets.buildRows(view.Theme())
	if err := view.Root().Add(rows); err != nil {
		t.Fatal(err)
	}

	return widgets, rows
}

func renderingRows(root *ui.Node) map[string]string {
	result := make(map[string]string)

	var group string

	for _, child := range root.Children() {
		if child.Kind() == ui.Text {
			group = child.Text()

			continue
		}

		row := child.Children()
		if len(row) == 2 {
			result[group+"/"+row[0].Text()] = row[1].Text()
		}
	}

	return result
}
