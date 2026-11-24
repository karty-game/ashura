package main

import (
	"context"
	"errors"

	"github.com/karty-game/ashura/pkg/engine"
	"github.com/karty-game/ashura/pkg/impostor"
	"github.com/karty-game/ashura/pkg/model"
	"github.com/karty-game/ashura/pkg/scene/resource"
	"github.com/karty-game/ashura/samples/internal/sample"
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
	cancel    context.CancelFunc
	pending   <-chan comparisonResult
}

type comparisonResult struct {
	example comparisonExample
	data    []byte
	err     error
}

func setupComparison(app *engine.App, world *engine.World, comparison *comparisonScene) (release func() error, err error) {
	presentation := &comparisonPresentation{
		app: app, world: world, scene: comparison, examples: comparisonExamples(), quality: impostor.DefaultQuality(),
	}
	presentation.requested = presentation.examples[0].id

	defer func() {
		if err != nil {
			sample.Cleanup(&err, presentation.close)
		}
	}()

	presentation.controls, err = showComparison(app, presentation)
	if err != nil {
		return nil, err
	}

	comparison.notify = func(lightIndex int, mode rotationMode) {
		presentation.failures.Check(presentation.controls.syncMotion(lightIndex, mode))
	}
	comparison.prepare = presentation.start

	return presentation.close, nil
}

func (presentation *comparisonPresentation) start() error {
	examples, err := comparisonExamplesForDevice(presentation.app)
	if err != nil {
		return err
	}

	presentation.examples = examples
	presentation.scene.prepare = presentation.update

	return presentation.update()
}

func (presentation *comparisonPresentation) update() error {
	if err := presentation.failures.Err(); err != nil {
		return err
	}

	if presentation.requested != "" {
		requested := presentation.requested
		presentation.requested = ""
		presentation.requestModel(requested)
	}

	select {
	case result := <-presentation.pending:
		presentation.pending = nil
		presentation.cancel()
		presentation.cancel = nil

		if result.err != nil {
			return result.err
		}

		return presentation.selectModel(result.example, result.data)
	default:
		return nil
	}
}

func (presentation *comparisonPresentation) requestModel(requested string) {
	if presentation.cancel != nil {
		presentation.cancel()
		presentation.cancel, presentation.pending = nil, nil
	}

	if requested == presentation.selected {
		return
	}

	example := selectExample(presentation.examples, requested)
	ctx, cancel := context.WithCancel(context.Background())
	results := make(chan comparisonResult, 1)
	presentation.cancel, presentation.pending = cancel, results

	go func() {
		data, err := resource.Read(ctx, resource.ProjectBase("samples/impostors"), example.card)
		results <- comparisonResult{example: example, data: data, err: err}
	}()
}

func (presentation *comparisonPresentation) selectModel(example comparisonExample, data []byte) (err error) {
	next := &comparisonResources{}

	defer func() {
		if err != nil && presentation.resources != next {
			sample.Cleanup(&err, next.close)
		}
	}()

	if err := next.load(presentation.app, presentation.world, example, data, presentation.quality); err != nil {
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
	if presentation.cancel != nil {
		presentation.cancel()
		presentation.cancel, presentation.pending = nil, nil
	}

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
	data []byte,
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

	resources.card, err = impostor.Prepared(example.card.ID, data).Load(app)
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
