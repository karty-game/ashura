//nolint:mnd // The preset origins and scales are authored in world units.
package main

import (
	"github.com/karty-game/ashura/pkg/engine"
	"github.com/karty-game/ashura/pkg/ui"
	"github.com/karty-game/ashura/samples/internal/sampleui"
	"github.com/karty-game/ashura/samples/particles/generated/assets"
)

const galleryCenterX = -1.2

func addGallery(app *engine.App, wall *engine.Transform) error {
	collections := galleryCollections()
	selected := collections[0].presets[0].key

	effects, err := prepareGallery(app, collections, selected)
	if err != nil {
		return err
	}

	replay := func() { effects[selected].Restart() }
	picker := galleryPicker(collections, func(key string) {
		effects[selected].Stop()
		selected = key
		wall.Position.X = galleryX(key) + wallOffsetX

		replay()
	})
	toggle := ui.NewSwitch("Wall", false, func(change ui.Change) {
		wall.Position.Y = loweredWallY
		if change.Checked {
			wall.Position.Y = wallY
		}

		replay()
	})

	playback := ui.Row(ui.NewButton("Replay effect", replay).WithStyle(ui.Style{Grow: 1}),
		toggle.WithStyle(ui.Style{Grow: 1})).WithStyle(ui.Style{Gap: 8})

	return sampleui.Controls(app, "Particle gallery", "Drag to orbit. Raise the wall to see it hide particles.", picker, playback)
}

func prepareGallery(app *engine.App, collections []galleryCollection, selected string) (map[string]*engine.ParticleInstance, error) {
	effects := make(map[string]*engine.ParticleInstance)

	for _, collection := range collections {
		for _, preset := range collection.presets {
			origin := engine.Vector3{X: galleryX(preset.key), Y: preset.height, Z: -.57}

			instance, err := addEffect(app, preset.key, origin, preset.scale)
			if err != nil {
				return nil, err
			}

			if preset.key != selected {
				instance.Stop()
			}

			effects[preset.key] = instance
		}
	}

	_, err := addEffect(app, "ion-plume", engine.Vector3{X: 2, Y: .34, Z: plumeZ}, .011)

	return effects, err
}

func galleryX(key string) float32 {
	switch key {
	case "water-splash", "waterfall-mist":
		return -.24 // Keep the low water effects clear of the controls.
	default:
		return galleryCenterX
	}
}

func galleryPicker(collections []galleryCollection, play func(string)) *ui.Node {
	options := make([]ui.Option, 0, len(collections))
	pickers := make(map[string]*ui.Node, len(collections))

	children := make([]*ui.Node, 1, len(collections)+1)
	active := collections[0].key

	for _, collection := range collections {
		options = append(options, ui.Option{Key: collection.key, Label: collection.label})

		picker := effectPicker(collection, play)
		picker.SetHidden(collection.key != active)
		pickers[collection.key] = picker
		children = append(children, picker)
	}

	children[0] = ui.NewSelect("Collection", options, active, func(change ui.Change) {
		pickers[active].SetHidden(true)
		active = change.Node.Selected()
		pickers[active].SetHidden(false)
		play(pickers[active].Selected())
	})

	return ui.Column(children...).WithStyle(ui.Style{Gap: 8})
}

func effectPicker(collection galleryCollection, play func(string)) *ui.Node {
	options := make([]ui.Option, 0, len(collection.presets))
	for _, preset := range collection.presets {
		options = append(options, ui.Option{Key: preset.key, Label: preset.label})
	}

	return ui.NewSelect("Effect", options, options[0].Key, func(change ui.Change) { play(change.Node.Selected()) })
}

func addEffect(app *engine.App, name string, origin engine.Vector3, scale float32) (*engine.ParticleInstance, error) {
	effect, err := assets.Load(name)
	if err != nil {
		return nil, err
	}

	return app.AddWorldParticles(effect, origin, scale)
}
