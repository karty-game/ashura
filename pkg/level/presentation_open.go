package level

import (
	"context"
	"errors"
	"slices"

	"github.com/karty-game/karty-webgpu/internal/leveldata"
	"github.com/karty-game/karty-webgpu/pkg/engine"
)

// Selection is an explicit snapshot of chunk and independent-object indices.
// Objects must name dynamic, published models with offline-flattened placement.
type Selection struct {
	Chunks  []int
	Objects []int
}

// OpenOptions chooses one prepared level and its application-owned residency.
type OpenOptions struct {
	Variants TextureVariants
	// Base is the existing resource reader's URL or filesystem root.
	Base string
	// Select runs once on the App loop after complete manifest admission. Treat
	// its manifest as read-only. Returned index slices are copied before I/O.
	Select func(Manifest) (Selection, error)
	// StatusChanged runs only on the App loop after progress changes. Close
	// emits no notification; Status exposes its Closed state explicitly.
	StatusChanged func(PresentationStatus)
}

// PresentationStatus contains stored counts, with no per-frame scanning.
type PresentationStatus struct {
	LoadedChunks, TotalChunks   int
	LoadedObjects, TotalObjects int
	Ready, Closed               bool
}

// Open establishes ownership without starting I/O or requiring an open device.
// Update selects the native profile after App.Run initializes the device.
// Select is mandatory: Open never infers portal reachability or preloads a world.
func Open(ctx context.Context, app *engine.App, world *engine.World, options OpenOptions) (*Presentation, error) {
	if ctx == nil || app == nil || world == nil || options.Base == "" || options.Select == nil {
		return nil, ErrFormat
	}

	if err := ctx.Err(); err != nil {
		return nil, err
	}

	if app.Closed() {
		return nil, errLifetime
	}

	owned, cancel := context.WithCancel(ctx)
	state := &Presentation{
		ctx: owned, cancel: cancel, app: app, world: world, options: options,
		loader: NewLoader(app), pending: make(chan presentationArrival, 1), independent: make(chan independentArrival, 1),
	}

	var err error

	state.closeCancel, err = app.OnClose(func() { _ = state.Close() })
	if err != nil {
		return nil, errors.Join(err, state.Close())
	}

	return state, nil
}

// WholeScene explicitly selects a complete single-space authored scene and its
// prepared independent models. Multi-space worlds must supply their own selector
// based on world/portal ownership or retain the lower-level Loader API.
func WholeScene(manifest Manifest) (Selection, error) {
	const maximumObjects = 8192 // Existing bounded level manifest contract.
	if len(manifest.Chunks) > leveldata.MaxChunks || len(manifest.Objects) > maximumObjects {
		return Selection{}, ErrFormat
	}

	for index, chunk := range manifest.Chunks {
		if index > 0 && chunk.Space != manifest.Chunks[0].Space {
			return Selection{}, ErrFormat
		}
	}

	selection := Selection{Chunks: make([]int, len(manifest.Chunks))}
	for index := range selection.Chunks {
		selection.Chunks[index] = index
	}

	for index, object := range manifest.Objects {
		if object.Dynamic {
			selection.Objects = append(selection.Objects, index)
		}
	}

	return selection, nil
}

func cloneSelection(selection Selection) Selection {
	return Selection{Chunks: slices.Clone(selection.Chunks), Objects: slices.Clone(selection.Objects)}
}

func validatePresentationSelection(manifest Manifest, selection Selection) error {
	if len(selection.Chunks) > len(manifest.Chunks) || len(selection.Objects) > len(manifest.Objects) {
		return ErrFormat
	}

	seen := make(map[int]bool, len(selection.Chunks))
	for _, index := range selection.Chunks {
		if index < 0 || index >= len(manifest.Chunks) || seen[index] {
			return ErrFormat
		}

		seen[index] = true
	}

	seen = make(map[int]bool, len(selection.Objects))
	for _, index := range selection.Objects {
		if index < 0 || index >= len(manifest.Objects) || seen[index] || !manifest.Objects[index].Dynamic {
			return ErrFormat
		}

		seen[index] = true
	}

	return nil
}
