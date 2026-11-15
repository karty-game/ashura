package ui_test

import (
	"encoding/binary"
	"math"
	"testing"
	"time"

	"github.com/karty-game/karty-webgpu/internal/uipaint"
	"github.com/karty-game/karty-webgpu/pkg/ui"
)

type submittedButtonRecord struct {
	texture *uipaint.Texture
	rect    ui.Rect
	clip    ui.Rect
	uv      ui.Rect
	mode    float32
}

func buttonRecords(t *testing.T, view *ui.View) []submittedButtonRecord {
	t.Helper()

	frame, err := view.Frame(time.Second)
	if err != nil {
		t.Fatal(err)
	}

	records := []submittedButtonRecord{}

	for _, batch := range frame.Batches {
		for index := range batch.Count {
			data := frame.Data[(batch.First+index)*uipaint.Stride:]
			value := func(offset int) float32 {
				return math.Float32frombits(binary.LittleEndian.Uint32(data[offset:]))
			}

			rect := func(offset int) ui.Rect {
				return ui.Rect{X: value(offset), Y: value(offset + 4), W: value(offset + 8), H: value(offset + 12)}
			}
			records = append(records, submittedButtonRecord{
				texture: batch.Texture, rect: rect(0), clip: rect(16), uv: rect(64), mode: value(88),
			})
		}
	}

	return records
}

func buttonImage(t *testing.T) *ui.ImageAsset {
	t.Helper()

	asset, err := ui.NewImageAsset(2, 1, []byte{255, 0, 0, 255, 0, 255, 0, 255})
	if err != nil {
		t.Fatal(err)
	}

	return asset
}

func TestButtonImageAndGlyphsRemainOneInteractionTarget(t *testing.T) {
	t.Parallel()

	activations := 0

	button := ui.NewButton("Equip", func() { activations++ })
	if err := button.SetButtonVisual(ui.ButtonVisual{Image: buttonImage(t), Size: 24}); err != nil {
		t.Fatal(err)
	}

	view := surface(t, ui.Column(button))

	var image, glyph submittedButtonRecord

	for _, record := range buttonRecords(t, view) {
		switch record.mode {
		case 1:
			image = record
		case 2:
			if glyph.texture == nil {
				glyph = record
			}
		}
	}

	if image.texture == nil || glyph.texture == nil || image.texture == glyph.texture || image.rect.X >= glyph.rect.X {
		t.Fatal("image/label did not retain distinct textures and leading geometry", image, glyph)
	}

	pointer(view, ui.PointerDown, image.rect.X+image.rect.W/2, image.rect.Y+image.rect.H/2)
	pointer(view, ui.PointerUp, image.rect.X+image.rect.W/2, image.rect.Y+image.rect.H/2)
	key(view, "Enter")

	if activations != 2 || view.Focused() != button {
		t.Fatal("image introduced another interaction target", activations, view.Focused())
	}

	semantics := view.Semantics()
	if len(semantics) != 2 || semantics[1].ID != button.ID() || semantics[1].Name != "Equip" {
		t.Fatal("decorative image changed the semantic tree", semantics)
	}
}

func TestButtonAtlasRegionsShareAnImmutableTexture(t *testing.T) {
	t.Parallel()

	pixels := []byte{255, 0, 0, 255, 0, 255, 0, 255, 0, 0, 255, 255, 255, 255, 0, 255}

	asset, err := ui.NewImageAsset(2, 2, pixels)
	if err != nil {
		t.Fatal(err)
	}

	pixels[0] = 0

	left, err := asset.Region(ui.Rect{W: 1, H: 2})
	if err != nil {
		t.Fatal(err)
	}

	right, err := asset.Region(ui.Rect{X: 1, W: 1, H: 2})
	if err != nil {
		t.Fatal(err)
	}

	first, second := ui.NewButton("Left", nil), ui.NewButton("Right", nil)
	if err := first.SetButtonVisual(ui.ButtonVisual{Image: left, Size: 24}); err != nil {
		t.Fatal(err)
	}

	if err := second.SetButtonVisual(ui.ButtonVisual{Image: right, Side: ui.TrailingContent, Size: 24}); err != nil {
		t.Fatal(err)
	}

	view := surface(t, ui.Column(first, second))
	images := []submittedButtonRecord{}

	for _, record := range buttonRecords(t, view) {
		if record.mode == 1 {
			images = append(images, record)
		}
	}

	if len(images) != 2 || images[0].texture != images[1].texture || images[0].texture.Pixels[0] != 255 {
		t.Fatal("atlas regions copied, lost their immutable pixels or failed to draw", images)
	}

	if images[0].uv.X != 0 || images[1].uv.X != 0.5 || images[0].uv.W != 0.5 || images[1].uv.W != 0.5 {
		t.Fatal("atlas region UVs did not select distinct halves", images)
	}

	for _, image := range images {
		if image.rect.W*2 != image.rect.H {
			t.Fatal("non-square atlas region was stretched", image.rect)
		}
	}
}

func TestHiddenButtonVisualReplacementPreservesVisibleBatches(t *testing.T) {
	t.Parallel()

	button := ui.NewButton("Hidden action", nil)
	if err := button.SetButtonVisual(ui.ButtonVisual{Image: buttonImage(t)}); err != nil {
		t.Fatal(err)
	}

	tabs := ui.NewTabs("Sections", []ui.Tab{
		{Key: "image", Label: "Image", Content: button},
		{Key: "plain", Label: "Plain", Content: ui.NewButton("Visible action", nil)},
	}, "image", nil)

	view := surface(t, ui.Column(tabs))
	if err := tabs.SetActiveTab("plain"); err != nil {
		t.Fatal(err)
	}

	paint(t, view)

	if err := button.SetButtonVisual(ui.ButtonVisual{Icon: "?", Side: ui.TrailingContent}); err != nil {
		t.Fatal(err)
	}

	for _, record := range buttonRecords(t, view) {
		if record.mode == 1 {
			t.Fatal("hidden replacement leaked an image batch into visible panel", record)
		}
	}

	if err := tabs.SetActiveTab("image"); err != nil {
		t.Fatal(err)
	}

	for _, record := range buttonRecords(t, view) {
		if record.mode == 1 {
			t.Fatal("retired image batch reappeared when hidden panel reopened", record)
		}
	}

	before := view.Stats()
	paint(t, view)

	if view.Stats() != before {
		t.Fatal("visual replacement did not return to retained idle", before, view.Stats())
	}
}

func TestMountedVisualBatchCountChangesPreserveSiblingPaint(t *testing.T) {
	t.Parallel()

	asset := buttonImage(t)
	button := ui.NewButton("Decorated", nil)
	sibling := ui.NewToggle("Sibling", false, nil)
	view := surface(t, ui.Column(button, sibling))

	for _, visual := range []ui.ButtonVisual{{Image: asset}, {Icon: "?"}, {Image: asset, Side: ui.TrailingContent}} {
		if err := button.SetButtonVisual(visual); err != nil {
			t.Fatal(err)
		}

		paint(t, view)
		sibling.SetChecked(!sibling.Checked())

		records := buttonRecords(t, view)
		images, siblingGlyphs := 0, 0

		for _, record := range records {
			if record.mode == 1 {
				images++
			}

			if record.mode == 2 && record.rect.Y >= sibling.Bounds().Y && record.rect.Y < sibling.Bounds().Y+sibling.Bounds().H {
				siblingGlyphs++
			}
		}

		wanted := 0
		if visual.Image != nil {
			wanted = 1
		}

		if images != wanted || siblingGlyphs != len("Sibling") {
			t.Fatal("batch-count change corrupted later paint", visual, images, siblingGlyphs)
		}
	}
}

func TestVisualOnlyTabKeepsItsAccessibleNameAndKeyboardSelection(t *testing.T) {
	t.Parallel()

	tabs := ui.NewTabs("Inventory", []ui.Tab{
		{Key: "items", Label: "Items", Visual: ui.ButtonVisual{Image: buttonImage(t)}, Content: ui.NewText("Items panel")},
		{Key: "help", AccessibleLabel: "Inventory help", Visual: ui.ButtonVisual{Icon: "?"}, Content: ui.NewText("Help panel")},
	}, "items", nil)
	view := surface(t, ui.Column(tabs))
	buttons := tabs.Children()[0].Children()
	view.SetFocus(buttons[0])
	key(view, "ArrowRight")
	paint(t, view)

	if tabs.ActiveTab() != "help" || view.Focused() != buttons[1] {
		t.Fatal("visual-only tab lost keyboard behavior", tabs.ActiveTab(), view.Focused())
	}

	found := false

	for _, semantic := range view.Semantics() {
		if semantic.ID == buttons[1].ID() {
			found = semantic.Role == "tab" && semantic.Name == "Inventory help" && semantic.Selected && semantic.Controls != 0
		}
	}

	if !found {
		t.Fatal("visual-only tab lost its name, role or selected relationship", view.Semantics())
	}
}
