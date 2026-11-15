package ui_test

import (
	"testing"
	"time"

	"github.com/karty-game/karty-webgpu/pkg/ui"
	"github.com/karty-game/karty-webgpu/pkg/ui/fonts/defaultfont"
)

func surface(t *testing.T, root *ui.Node) *ui.View {
	t.Helper()

	font, err := defaultfont.Load()
	if err != nil {
		t.Fatal(err)
	}

	view, err := ui.New(root, font)
	if err != nil {
		t.Fatal(err)
	}

	if err := view.Resize(500, 400); err != nil {
		t.Fatal(err)
	}

	paint(t, view)

	return view
}
func paint(t *testing.T, view *ui.View) {
	t.Helper()

	if _, err := view.Frame(time.Second); err != nil {
		t.Fatal(err)
	}
}
func point(node *ui.Node) (float32, float32) {
	r := node.Bounds()

	return r.X + r.W/2, r.Y + r.H/2
}
func pointer(view *ui.View, kind ui.EventKind, positionX, positionY float32) {
	view.Dispatch(ui.Event{Kind: kind, Pointer: 1, Device: "mouse", X: positionX, Y: positionY})
}
func click(view *ui.View, node *ui.Node) {
	positionX, positionY := point(node)
	pointer(view, ui.PointerDown, positionX, positionY)
	pointer(view, ui.PointerUp, positionX, positionY)
}
func key(view *ui.View, key string) {
	view.Dispatch(ui.Event{Kind: ui.KeyDown, Key: key})
	view.Dispatch(ui.Event{Kind: ui.KeyUp, Key: key})
}

func TestPressCancellationAndReentry(t *testing.T) {
	t.Parallel()

	count := 0
	button := ui.NewButton("Run", func() { count++ })
	view := surface(t, ui.Column(button))
	positionX, positionY := point(button)
	pointer(view, ui.PointerDown, positionX, positionY)
	pointer(view, ui.PointerMove, 510, 390)
	pointer(view, ui.PointerUp, 510, 390)

	if count != 0 {
		t.Fatal("release outside activated")
	}

	pointer(view, ui.PointerDown, positionX, positionY)
	pointer(view, ui.PointerMove, 510, 390)
	pointer(view, ui.PointerMove, positionX, positionY)
	pointer(view, ui.PointerUp, positionX, positionY)

	if count != 1 {
		t.Fatal("returning before release did not activate once", count)
	}

	pointer(view, ui.PointerDown, positionX, positionY)
	pointer(view, ui.PointerCancel, positionX, positionY)
	pointer(view, ui.PointerUp, positionX, positionY)

	if count != 1 || view.CapturesPointer() {
		t.Fatal("cancellation retained activation/capture")
	}

	key(view, " ")
	key(view, "Enter")

	if count != 3 {
		t.Fatal("keyboard activation", count)
	}
}
func TestDisableAndRemovalDuringDispatch(t *testing.T) {
	t.Parallel()

	count := 0
	button := ui.NewButton("Remove", func() { count++ })
	root := ui.Column(button)
	view := surface(t, root)
	positionX, positionY := point(button)
	pointer(view, ui.PointerDown, positionX, positionY)
	button.SetDisabled(true)
	pointer(view, ui.PointerUp, positionX, positionY)

	if count != 0 || view.Focused() != nil || view.CapturesPointer() {
		t.Fatal("disable did not cancel")
	}

	button.SetDisabled(false)
	button.OnPreview(func(e *ui.RoutedEvent) {
		if e.Event.Kind == ui.PointerDown {
			button.Remove()
		}
	})
	pointer(view, ui.PointerDown, positionX, positionY)
	pointer(view, ui.PointerUp, positionX, positionY)

	if count != 0 || view.Lookup(button.ID()) != nil {
		t.Fatal("removed target received default action")
	}
}
func TestNestedTouchScrollCancelsPress(t *testing.T) {
	t.Parallel()

	count := 0
	button := ui.NewButton("Item", func() { count++ })

	children := make([]*ui.Node, 0, 11)

	children = append(children, button)
	for range 10 {
		children = append(children, ui.NewButton("More", nil))
	}

	root := ui.Column(children...).WithStyle(ui.Style{Height: 150, Scroll: true})
	view := surface(t, ui.Column(root))
	positionX, positionY := point(button)
	view.Dispatch(ui.Event{Kind: ui.PointerDown, Pointer: 8, Device: "touch", X: positionX, Y: positionY})
	view.Dispatch(ui.Event{Kind: ui.PointerMove, Pointer: 8, Device: "touch", X: positionX, Y: positionY - 35})
	view.Dispatch(ui.Event{Kind: ui.PointerUp, Pointer: 8, Device: "touch", X: positionX, Y: positionY - 35})
	paint(t, view)

	if count != 0 || button.Bounds().Y >= 0 || view.CapturesPointer() {
		t.Fatal("touch scrolling clicked or failed to scroll", count, button.Bounds())
	}
}
func TestSelectPreviewAndAcceptance(t *testing.T) {
	t.Parallel()

	changes := 0
	node := ui.NewSelect(
		"Preset",
		[]ui.Option{{Key: "a", Label: "Alpha"}, {Key: "b", Label: "Blocked", Disabled: true}, {Key: "c", Label: "Charlie"}},
		"a",
		func(ui.Change) { changes++ },
	)
	view := surface(t, ui.Column(node))
	click(view, node)
	paint(t, view)
	key(view, "ArrowDown")

	if node.Selected() != "a" || changes != 0 {
		t.Fatal("navigation committed preview")
	}

	key(view, "Escape")

	if node.Selected() != "a" {
		t.Fatal("escape committed")
	}

	click(view, node)
	paint(t, view)
	key(view, "ArrowDown")
	key(view, "Enter")

	if node.Selected() != "c" || changes != 1 {
		t.Fatal("disabled option not skipped or commit duplicated", node.Selected(), changes)
	}
}
func TestSliderAndSilentSetters(t *testing.T) {
	t.Parallel()

	changes, commits := 0, 0
	node := ui.NewSlider("Volume", 0, 100, 5, 50, func(ui.Change) { changes++ })
	node.OnCommit(func(ui.Change) { commits++ })
	view := surface(t, ui.Column(node))
	view.SetFocus(node)
	key(view, "ArrowRight")
	key(view, "End")

	if node.Value() != 100 || changes != 2 || commits != 2 {
		t.Fatal("range keyboard contract", node.Value(), changes, commits)
	}

	if err := node.SetValue(10); err != nil {
		t.Fatal(err)
	}

	if changes != 2 {
		t.Fatal("programmatic setter emitted change")
	}

	positionX, positionY := point(node)
	pointer(view, ui.PointerDown, positionX, positionY)
	pointer(view, ui.PointerCancel, positionX, positionY)

	if commits != 2 || view.CapturesPointer() {
		t.Fatal("canceled slider committed")
	}
}
func TestFocusScopeAndPaintDamage(t *testing.T) {
	t.Parallel()

	first := ui.NewButton("A", nil)
	second := ui.NewButton("B", nil)
	scope := ui.Column(second)
	view := surface(t, ui.Column(first, scope))
	key(view, "Tab")

	if view.Focused() != first {
		t.Fatal("initial tab")
	}

	if err := view.PushModal(scope); err != nil {
		t.Fatal(err)
	}

	key(view, "Tab")

	if view.Focused() != second {
		t.Fatal("scope escaped")
	}

	view.PopModal()

	if view.Focused() != first {
		t.Fatal("focus was not restored")
	}

	paint(t, view)
	stats := view.Stats()

	frame, err := view.Frame(time.Second)
	if err != nil {
		t.Fatal(err)
	}

	revision := frame.Revision

	for range 60 {
		paint(t, view)
	}

	if view.Stats() != stats || frame.Revision != revision {
		t.Fatal("idle interface rebuilt")
	}

	positionX, positionY := point(first)
	pointer(view, ui.PointerMove, positionX, positionY)
	paint(t, view)

	if view.Stats().Layouts != stats.Layouts || view.Stats().Shapes != stats.Shapes || view.Stats().Paints != stats.Paints+1 {
		t.Fatal("hover rebuilt layout/text or siblings", view.Stats(), stats)
	}
}
