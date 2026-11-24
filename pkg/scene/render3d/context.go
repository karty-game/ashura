// Package render3d connects generated logical scenes to existing prepared models.
// Resource arrival and view visibility never construct or destroy gameplay state.
package render3d

import (
	"errors"
	"fmt"

	"github.com/karty-game/ashura/pkg/engine"
	"github.com/karty-game/ashura/pkg/model"
	"github.com/karty-game/ashura/pkg/scene"
)

var (
	// ErrInvalid marks a missing World, model, asset identity or render context.
	ErrInvalid = errors.New("invalid scene render resource")
	// ErrSupplied means this context already owns a model under the identity.
	// Evict that resource before supplying its replacement.
	ErrSupplied = errors.New("scene render resource is already supplied")

	errResource = ErrInvalid
)

// Context owns shared prepared model handles for one World and application.
// Supply and all scene mutations run on the application's loop goroutine.
type Context struct {
	World  *engine.World
	models map[string]*model.Model
	views  map[string]map[*ModelView]struct{}
	closed bool
}

// NewContext starts resource ownership without loading any model bytes.
func NewContext(world *engine.World) *Context {
	return &Context{World: world, models: make(map[string]*model.Model), views: make(map[string]map[*ModelView]struct{})}
}

// ModelView is a logical model placement whose render entities can be absent.
type ModelView struct {
	context   *Context
	root      *scene.Root
	asset     string
	transform engine.Transform
	instance  *model.Instance
	visible   bool
	baked     bool
}

// NewModel registers a placement. It never fetches or transforms asset bytes.
func NewModel(root *scene.Root, context *Context, asset string, transform engine.Transform) (*ModelView, error) {
	return newModel(root, context, asset, transform, false)
}

// NewBakedModel retains a logical placement represented by compiled level chunks.
// Supplying its original model cannot instantiate duplicate source geometry.
// SetVisible preserves logical visibility only; static chunk geometry is immutable.
func NewBakedModel(root *scene.Root, context *Context, asset string, transform engine.Transform) (*ModelView, error) {
	return newModel(root, context, asset, transform, true)
}

func newModel(root *scene.Root, context *Context, asset string, transform engine.Transform, baked bool) (*ModelView, error) {
	if root.Closed() || context != nil && context.closed {
		return nil, scene.ErrClosed
	}

	if context == nil || context.World == nil || asset == "" {
		return nil, errResource
	}

	view := &ModelView{context: context, root: root, asset: asset, transform: transform, visible: true, baked: baked}
	if context.views[asset] == nil {
		context.views[asset] = make(map[*ModelView]struct{})
	}

	context.views[asset][view] = struct{}{}
	if err := view.attach(); err != nil {
		delete(context.views[asset], view)

		return nil, err
	}

	if err := root.Own(view.close); err != nil {
		view.close()

		return nil, err
	}

	return view, nil
}

// SetVisible attaches/removes presentation only. The root and component state remain alive.
func (view *ModelView) SetVisible(visible bool) error {
	if view.root.Closed() || view.context.closed {
		return scene.ErrClosed
	}

	view.visible = visible
	if !visible && view.instance != nil {
		view.instance.Remove()
		view.instance = nil
	}

	return view.attach()
}

// Resident reports whether this placement currently has render entities.
func (view *ModelView) Resident() bool { return view.instance != nil }

func (view *ModelView) attach() error {
	if view.baked || !view.visible || view.root.Closed() || view.instance != nil {
		return nil
	}

	asset := view.context.models[view.asset]
	if asset == nil {
		return nil
	}

	instance, err := asset.Instantiate(view.context.World, view.transform)
	if err != nil {
		return err
	}

	view.instance = instance

	return nil
}

func (view *ModelView) close() {
	if view.instance != nil {
		view.instance.Remove()
		view.instance = nil
	}

	delete(view.context.views[view.asset], view)

	if len(view.context.views[view.asset]) == 0 {
		delete(view.context.views, view.asset)
	}
}

// Supply transfers ownership of an already-loaded model and activates its waiting views.
// Only views dependent on this resource are visited. Failed activation rolls back.
func (context *Context) Supply(identity string, asset *model.Model) error {
	if context == nil {
		return errResource
	}

	if context.closed {
		return scene.ErrClosed
	}

	if asset == nil || identity == "" {
		return errResource
	}

	if context.models[identity] != nil {
		return fmt.Errorf("supply %q: %w", identity, ErrSupplied)
	}

	context.models[identity] = asset
	for view := range context.views[identity] {
		if err := view.attach(); err != nil {
			for previous := range context.views[identity] {
				if previous.instance != nil {
					previous.instance.Remove()
					previous.instance = nil
				}
			}

			delete(context.models, identity)

			return err
		}
	}

	return nil
}

// Evict removes this resource's render entities and releases the model, preserving views/state.
func (context *Context) Evict(identity string) error {
	asset := context.models[identity]
	if asset == nil {
		return nil
	}

	for view := range context.views[identity] {
		if view.instance != nil {
			view.instance.Remove()
			view.instance = nil
		}
	}

	if err := asset.Unload(); err != nil {
		return fmt.Errorf("evict %s: %w", identity, err)
	}

	delete(context.models, identity)

	return nil
}

// Close releases all model ownership, preserving logical roots for explicit teardown.
func (context *Context) Close() error {
	for identity := range context.models {
		if err := context.Evict(identity); err != nil {
			return err
		}
	}

	context.closed = true

	return nil
}

// SetVisible changes all current model presentations on an explicit user action.
// This is not called by the frame loop and does not alter any gameplay component.
func (context *Context) SetVisible(visible bool) error {
	for _, views := range context.views {
		for view := range views {
			if err := view.SetVisible(visible); err != nil {
				return err
			}
		}
	}

	return nil
}
