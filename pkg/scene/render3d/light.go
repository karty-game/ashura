package render3d

import (
	"github.com/karty-game/ashura/pkg/engine"
	"github.com/karty-game/ashura/pkg/light"
	"github.com/karty-game/ashura/pkg/scene"
)

// Add attaches a render component with root-owned cleanup and no asset preparation.
func Add(root *scene.Root, context *Context, transform engine.Transform, component any) (engine.Entity, error) {
	if root.Closed() || context != nil && context.closed {
		return engine.Entity{}, scene.ErrClosed
	}

	if context == nil || context.World == nil {
		return engine.Entity{}, errResource
	}

	entity := context.World.Add(transform, component)
	if err := root.Own(func() { context.World.Remove(entity) }); err != nil {
		context.World.Remove(entity)

		return engine.Entity{}, err
	}

	return entity, nil
}

// Point connects a light's presentation to a root-owned observable boolean.
// Toggling it updates only the light component's existing intensity field.
func Point(
	root *scene.Root,
	context *Context,
	transform engine.Transform,
	source light.Point,
	enabled *scene.Value[bool],
) (engine.Entity, error) {
	intensity := source.Intensity

	source.Position = transform.Position
	if !enabled.Get() {
		source.Intensity = 0
	}

	entity, err := Add(root, context, transform, source)
	if err != nil {
		return engine.Entity{}, err
	}

	unsubscribe := enabled.Subscribe(func(on bool) {
		component, found := engine.Get[light.Point](context.World, entity)
		if found {
			component.Intensity = 0
			if on {
				component.Intensity = intensity
			}
		}
	})
	if err := root.Own(unsubscribe); err != nil {
		unsubscribe()

		return engine.Entity{}, err
	}

	return entity, nil
}
