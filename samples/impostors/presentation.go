package main

import (
	"errors"

	"github.com/karty-game/karty-webgpu/pkg/engine"
	"github.com/karty-game/karty-webgpu/pkg/impostor"
	"github.com/karty-game/karty-webgpu/pkg/model"
	"github.com/karty-game/karty-webgpu/samples/internal/sample"
)

type comparisonResources struct {
	full  *model.Model
	card  *impostor.Impostor
	left  *model.Instance
	right *impostor.Instance
}

type comparisonPresentation struct {
	app       *engine.App
	world     *engine.World
	scene     *comparisonScene
	resources *comparisonResources
	controls  *comparisonUI
	examples  [3]comparisonExample
	selected  string
	requested string
	quality   impostor.Quality
	failures  sample.Errors
}

func setupComparison(app *engine.App, world *engine.World, comparison *comparisonScene) (release func() error, err error) {
	presentation := &comparisonPresentation{
		app: app, world: world, scene: comparison, examples: comparisonExamples(), quality: impostor.DefaultQuality(),
	}

	defer func() {
		if err != nil {
			sample.Cleanup(&err, presentation.close)
		}
	}()

	if err := presentation.selectModel(presentation.examples[0].id); err != nil {
		return nil, err
	}

	presentation.controls, err = showComparison(app, presentation)
	if err != nil {
		return nil, err
	}

	comparison.notify = func(lightIndex int, mode rotationMode) {
		presentation.failures.Check(presentation.controls.syncMotion(lightIndex, mode))
	}
	comparison.prepare = presentation.update

	return presentation.close, nil
}

func (presentation *comparisonPresentation) update() error {
	if err := presentation.failures.Err(); err != nil {
		return err
	}

	if presentation.requested == "" {
		return nil
	}

	requested := presentation.requested
	presentation.requested = ""

	return presentation.selectModel(requested)
}

func (presentation *comparisonPresentation) selectModel(id string) (err error) {
	if id == presentation.selected {
		return nil
	}

	example := selectExample(presentation.examples, id)

	next := &comparisonResources{}

	defer func() {
		if err != nil && presentation.resources != next {
			sample.Cleanup(&err, next.close)
		}
	}()

	if err := next.load(presentation.app, presentation.world, example, presentation.quality); err != nil {
		return err
	}

	previous := presentation.resources
	presentation.resources, presentation.selected = next, example.id
	entities := next.left.Entities()

	transforms := make([]*engine.Transform, 0, len(entities))
	for _, entity := range entities {
		transforms = append(transforms, presentation.world.Transform(entity))
	}

	presentation.scene.attach(transforms, presentation.world.Transform(next.right.Entity()), next.card.Center())
	presentation.scene.qualityAsset = next.card
	// load already applied the latest accepted quality to this pair. Discard
	// requests queued for its predecessor, including a different capture mode.
	presentation.scene.qualityPending = false

	if previous != nil {
		if err := previous.close(); err != nil {
			return err
		}
	}

	if presentation.controls != nil {
		return presentation.controls.syncModel(example, next.card)
	}

	return nil
}

func (presentation *comparisonPresentation) close() error {
	presentation.scene.prepare, presentation.scene.notify = nil, nil
	presentation.scene.qualityAsset = nil

	presentation.scene.modelTransforms, presentation.scene.impostorTransform = nil, nil

	if presentation.controls != nil {
		presentation.controls.remove()
		presentation.controls = nil
	}

	if presentation.resources == nil {
		return nil
	}

	err := presentation.resources.close()
	presentation.resources = nil

	return err
}

func (resources *comparisonResources) load(
	app *engine.App,
	world *engine.World,
	example comparisonExample,
	quality impostor.Quality,
) error {
	var err error

	resources.full, err = example.model.Load(app)
	if err != nil {
		return err
	}

	resources.left, err = resources.full.Instantiate(world,
		engine.Transform{Position: engine.Vector3{X: -pairOffset}, Scale: engine.Vector3One()})
	if err != nil {
		return err
	}

	resources.card, err = example.card.Load(app)
	if err != nil {
		return err
	}

	if resources.card.SupportsQuality() {
		if err := resources.card.SetQuality(quality); err != nil {
			return err
		}
	}

	resources.right, err = resources.card.Instantiate(world,
		engine.Transform{Position: engine.Vector3{X: pairOffset}, Scale: engine.Vector3One()})

	return err
}

func (resources *comparisonResources) close() error {
	resources.right.Remove()
	resources.left.Remove()

	return errors.Join(resources.card.Unload(), resources.full.Unload())
}
