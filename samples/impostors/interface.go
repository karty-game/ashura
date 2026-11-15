//nolint:mnd // The comparison panel uses authored logical-pixel dimensions and quality ranges.
package main

import (
	"errors"
	"fmt"
	"strconv"

	"github.com/karty-game/karty-webgpu/pkg/engine"
	"github.com/karty-game/karty-webgpu/pkg/impostor"
	"github.com/karty-game/karty-webgpu/pkg/ui"
	"github.com/karty-game/karty-webgpu/pkg/ui/app"
	"github.com/karty-game/karty-webgpu/pkg/ui/fonts/defaultfont"
	"github.com/karty-game/karty-webgpu/samples/internal/sampleui"
)

const (
	qualityAuto     = "auto"
	qualityHigh     = "high"
	qualityStandard = "standard"
	qualityLow      = "low"
)

type comparisonUI struct {
	presentation       *comparisonPresentation
	counts, status     *ui.Node
	light, rotation    *ui.Node
	quality, far, near *ui.Node
	mip, qualityGroup  *ui.Node
	qualityToggle      *ui.Node
	qualityExpanded    bool
	farText, nearText  string
	mode               string
	mipBias            float32
	remove             func()
}

func showComparison(app *engine.App, presentation *comparisonPresentation) (*comparisonUI, error) {
	controls, panel := newComparisonUI(presentation)

	if err := errors.Join(
		controls.syncModel(selectExample(presentation.examples, presentation.selected), presentation.resources.card),
		controls.syncMotion(presentation.scene.state.lightIndex, presentation.scene.state.mode),
	); err != nil {
		return nil, err
	}

	view := uiapp.View(app)
	if view == nil {
		font, err := defaultfont.Load()
		if err != nil {
			return nil, err
		}

		view, err = ui.New(ui.Column(), font)
		if err != nil {
			return nil, err
		}

		if err := uiapp.Attach(app, view); err != nil {
			return nil, err
		}
	}

	remove, err := view.AddOverlay(panel, ui.OverlayOptions{
		Anchor: ui.OverlayBottomLeft, Width: 720, MaxHeightRatio: .45, Margin: 12, Cursor: true,
	})
	if err != nil {
		return nil, err
	}

	controls.remove = remove

	return controls, nil
}

func newComparisonUI(presentation *comparisonPresentation) (*comparisonUI, *ui.Node) {
	settings := presentation.quality
	controls := &comparisonUI{
		presentation: presentation, counts: ui.NewText(""), status: ui.NewText(""), mode: qualityAuto,
		farText: fmt.Sprint(settings.FarPixels), nearText: fmt.Sprint(settings.NearPixels), mipBias: settings.MipBias,
	}

	options := make([]ui.Option, 0, len(presentation.examples))
	for _, example := range presentation.examples {
		options = append(options, ui.Option{Key: example.id, Label: example.label})
	}

	models := ui.NewSelect("Model", options, presentation.selected, func(change ui.Change) {
		presentation.requested = change.Text
	}).WithStyle(ui.Style{Width: 300, Grow: 1})
	controls.light = ui.NewButton("", presentation.scene.requestLight).WithStyle(ui.Style{Width: 190})
	controls.rotation = ui.NewButton("", presentation.scene.requestRotation).WithStyle(ui.Style{Width: 190})
	controls.buildQuality()
	controls.qualityToggle = ui.NewSwitch("Reprojection controls", false, func(change ui.Change) {
		controls.qualityExpanded = change.Checked
		controls.qualityGroup.SetHidden(!change.Checked || !presentation.resources.card.SupportsQuality())
	}).WithStyle(ui.Style{Width: 220})
	panel := ui.Column(
		ui.NewText("Full model  /  Impostor").WithStyle(ui.Style{FontSize: 20}),
		controls.counts,
		ui.Row(models, controls.qualityToggle).WithStyle(ui.Style{Wrap: true, Gap: 8}),
		sampleui.Details("Light, rotation and view", ui.Row(controls.light, controls.rotation,
			ui.NewButton("Reset view · R", presentation.scene.requestReset).WithStyle(ui.Style{Width: 160})).
			WithStyle(ui.Style{Wrap: true, Gap: 6}), ui.NewButton("Inspect close", func() {
			presentation.scene.orbit.Distance = 8
			positionCamera(presentation.scene.orbit, presentation.scene.view)
		})),
		controls.qualityGroup,
		ui.NewText("Left: full model · Right: impostor. Drag to orbit; wheel / pinch to zoom. Close views expose approximation errors."),
	).WithLabel("Model and impostor comparison", "Matched models, lighting, rotation and impostor quality").
		WithStyle(ui.Style{Padding: 12, Gap: 8, Radius: 12, Fill: ui.DarkTheme().Surface, Scroll: true})

	return controls, panel
}

func (controls *comparisonUI) buildQuality() {
	controls.quality = ui.NewSelect("Reprojection", []ui.Option{
		{Key: qualityAuto, Label: "Auto"}, {Key: qualityHigh, Label: "2 corrections"},
		{Key: qualityStandard, Label: "1 correction"}, {Key: qualityLow, Label: "0 corrections"},
	}, qualityAuto, func(change ui.Change) {
		controls.mode = change.Text
		controls.applyQuality()
	})
	controls.far = ui.NewInput("Far threshold (px)", controls.farText, "16", func(change ui.Change) {
		controls.farText = change.Text
		if !change.Composing {
			controls.applyQuality()
		}
	})
	controls.near = ui.NewInput("Near threshold (px)", controls.nearText, "128", func(change ui.Change) {
		controls.nearText = change.Text
		if !change.Composing {
			controls.applyQuality()
		}
	})
	controls.mip = ui.NewSlider("Mip bias", -4, 4, .5, float64(controls.mipBias), func(change ui.Change) {
		controls.mipBias = float32(change.Value)
		controls.applyQuality()
	})
	controls.qualityGroup = ui.Column(
		ui.Row(
			ui.Column(ui.NewText("Mode"), controls.quality).WithStyle(ui.Style{Width: 180, Grow: 1, Gap: 4}),
			ui.Column(ui.NewText("Far (px)"), controls.far).WithStyle(ui.Style{Width: 160, Grow: 1, Gap: 4}),
			ui.Column(ui.NewText("Near (px)"), controls.near).WithStyle(ui.Style{Width: 160, Grow: 1, Gap: 4}),
		).WithStyle(ui.Style{Wrap: true, Gap: 8}),
		ui.NewText("Mip bias"), controls.mip, controls.status,
	).WithStyle(ui.Style{Gap: 6})
	controls.qualityGroup.SetHidden(true)
}

func (controls *comparisonUI) syncMotion(lightIndex int, mode rotationMode) error {
	return errors.Join(
		controls.light.SetText(fmt.Sprintf("Light: %d/3 · L", lightIndex+1)),
		controls.rotation.SetText("Rotate: "+mode.label()+" · T"),
	)
}

func (controls *comparisonUI) syncModel(example comparisonExample, asset *impostor.Impostor) error {
	controls.qualityToggle.SetHidden(!asset.SupportsQuality())
	controls.qualityGroup.SetHidden(!asset.SupportsQuality() || !controls.qualityExpanded)

	return controls.counts.SetText(fmt.Sprintf("%s · full model: %d triangles · impostor: %d triangles",
		example.label, asset.Triangles(), asset.ProxyTriangles()))
}

func (controls *comparisonUI) applyQuality() {
	settings, err := controls.settings()
	controls.far.SetDisabled(controls.mode != qualityAuto)
	controls.near.SetDisabled(controls.mode != qualityAuto)

	if err == nil {
		err = controls.presentation.scene.requestQuality(settings)
	}

	if err != nil {
		controls.presentation.failures.Check(controls.status.SetText("Use finite values; Near must be at least twice Far."))

		return
	}

	controls.presentation.quality = settings
	controls.presentation.failures.Check(controls.status.SetText(fmt.Sprintf("Mip %+.1f", settings.MipBias)))
}

func (controls *comparisonUI) settings() (impostor.Quality, error) {
	settings := controls.presentation.quality

	settings.Adaptive, settings.MipBias = controls.mode == qualityAuto, controls.mipBias
	if settings.Adaptive {
		settings.MaxCorrections = 2
		far, farErr := strconv.ParseFloat(controls.farText, 32)

		near, nearErr := strconv.ParseFloat(controls.nearText, 32)
		if err := errors.Join(farErr, nearErr); err != nil {
			return settings, err
		}

		settings.FarPixels, settings.NearPixels = float32(far), float32(near)
	} else {
		switch controls.mode {
		case qualityHigh:
			settings.MaxCorrections = 2
		case qualityStandard:
			settings.MaxCorrections = 1
		case qualityLow:
			settings.MaxCorrections = 0
		}
	}

	return settings, settings.Validate()
}
