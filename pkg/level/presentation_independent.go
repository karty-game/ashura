package level

import (
	"fmt"

	"github.com/karty-game/karty-webgpu/pkg/engine"
	"github.com/karty-game/karty-webgpu/pkg/model"
	"github.com/karty-game/karty-webgpu/pkg/scene/resource"
)

var (
	errIndependentReference = fmt.Errorf("independent model references conflict or are missing: %w", ErrFormat)
	errIndependentTransform = fmt.Errorf("independent model placements must be flattened offline: %w", ErrFormat)
)

type independentGroup struct {
	asset   resource.Asset
	objects []Object
}

type independentArrival struct {
	group   independentGroup
	payload *resource.Payload
	err     error
}

type independentInstance struct {
	objectID string
	instance *model.Instance
}

func independentGroups(manifest *Manifest, selected []int) ([]independentGroup, error) {
	var groups []independentGroup

	resources := make(map[string]int)
	identities := make(map[string]Reference)
	identity := [16]float32{1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1}

	for _, index := range selected {
		object := manifest.Objects[index]

		if object.Asset == nil || object.Model == "" {
			return nil, errIndependentReference
		}

		if object.Transform != identity {
			return nil, errIndependentTransform
		}

		if previous, exists := identities[object.Model]; exists &&
			(previous.SHA256 != object.Asset.SHA256 || previous.Size != object.Asset.Size) {
			return nil, errIndependentReference
		}

		identities[object.Model] = *object.Asset

		groupIndex, exists := resources[object.Asset.SHA256]
		if !exists {
			groupIndex = len(groups)
			groups = append(groups, independentGroup{asset: resource.Asset{
				ID: object.Model, Path: object.Asset.Path, Size: object.Asset.Size, SHA256: object.Asset.SHA256,
			}})
			resources[object.Asset.SHA256] = groupIndex
		}

		if groups[groupIndex].asset.Size != object.Asset.Size {
			return nil, errIndependentReference
		}

		groups[groupIndex].objects = append(groups[groupIndex].objects, object)
	}

	return groups, nil
}

// Selected independent models retain build-prepared placement and exact content
// sharing. No source name, material grouping or coordinate reachability is inferred.
func (state *Presentation) requestIndependent() error {
	groups, err := independentGroups(state.manifest, state.selection.Objects)
	if err != nil {
		return err
	}

	if len(groups) == 0 {
		state.independentReady = true

		return nil
	}

	state.reads.Go(func() {
		defer close(state.independent)

		for _, group := range groups {
			payload, err := resource.ReadVerified(state.ctx, state.options.Base, group.asset)
			select {
			case state.independent <- independentArrival{group: group, payload: payload, err: err}:
			case <-state.ctx.Done():
				return
			}

			if err != nil {
				return
			}
		}
	})

	return nil
}

func (state *Presentation) pollIndependent() error {
	if state.independentReady {
		return nil
	}

	select {
	case item, open := <-state.independent:
		if !open {
			state.independentReady = true
			state.notify()

			return nil
		}

		if item.err != nil {
			return item.err
		}

		asset, err := model.Verified(item.group.asset.ID, item.payload).Load(state.app)
		if err != nil {
			return err
		}

		state.models = append(state.models, asset)
		for _, object := range item.group.objects {
			instance, err := asset.Instantiate(state.world, engine.DefaultTransform())
			if err != nil {
				return err
			}

			state.modelInstances = append(state.modelInstances, independentInstance{objectID: object.ID, instance: instance})
		}

		state.notify()
	default:
	}

	return nil
}
