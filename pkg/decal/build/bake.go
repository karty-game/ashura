//go:build !js

package decalbuild

import (
	"cmp"
	"fmt"
	"slices"

	"github.com/karty-game/ashura/internal/decaldata"
	"github.com/karty-game/ashura/pkg/assetbuild"
	"github.com/karty-game/ashura/pkg/engine"
)

// Triangle maps one world-space, front-facing receiver triangle to unique
// normalized albedo UVs. Order the positions counter-clockwise from the front.
type Triangle struct {
	Positions [3]engine.Vector3
	UVs       [3]engine.Vector2
}

// BakeOptions selects one standalone receiver albedo and its unique UVs.
// Source marks are eligible where ReceiverMask intersects Layers.
type BakeOptions struct {
	Receiver  string
	Layers    uint32
	Triangles []Triangle
	Marks     []Mark
}

type bakePlugin struct{ options BakeOptions }

// BakeAlbedo composites permanent projected marks before mip generation.
func BakeAlbedo(options BakeOptions) assetbuild.Plugin { return bakePlugin{options: options} }

func (bakePlugin) Name() string { return "decal-albedo" }

func (plugin bakePlugin) Plan(plan *assetbuild.Plan) error {
	options := plugin.options
	if options.Layers == 0 || len(options.Marks) == 0 || len(options.Marks) > decaldata.MaxBatch || len(options.Triangles) == 0 {
		return decaldata.ErrProjector
	}

	index, receiver, err := imageReference(plan, options.Receiver)
	if err != nil {
		return err
	}

	inputs := []assetbuild.Ref{receiver}

	references := make(map[string]assetbuild.Ref)
	for _, mark := range options.Marks {
		if _, exists := references[mark.Image]; exists {
			continue
		}

		_, source, err := imageReference(plan, mark.Image)
		if err != nil {
			return err
		}

		references[mark.Image] = source
		if source != receiver {
			inputs = append(inputs, source)
		}
	}

	refs, err := plan.Add(assetbuild.Action{
		ID: "decal-albedo/" + options.Receiver, Processor: plugin.Name(), Revision: "projection@1", Options: options,
		Inputs: inputs, Parallel: true, Outputs: []assetbuild.Slot{{Name: "data", Schema: assetbuild.ImageSetSchema}},
		Run: func(ctx *assetbuild.Context) (assetbuild.Result, error) {
			return bakeImages(ctx, options, receiver, references)
		},
	})
	if err != nil {
		return err
	}

	plan.Resources[index].Ref = refs[0]

	return nil
}

type bakedMark struct {
	image      assetbuild.Image
	projection decaldata.Projection
}

func bakeImages(
	ctx *assetbuild.Context, options BakeOptions, receiver assetbuild.Ref, sources map[string]assetbuild.Ref,
) (assetbuild.Result, error) {
	base, err := readBakeImage(ctx, receiver)
	if err != nil {
		return nil, err
	}

	marks, err := loadBakeMarks(ctx, options, sources)
	if err != nil {
		return nil, err
	}

	if err := paintAlbedo(ctx, &base, options.Triangles, marks); err != nil {
		return nil, fmt.Errorf("receiver %q: %w", options.Receiver, err)
	}

	result, err := assetbuild.EncodeImageSet(assetbuild.ImageSet{Images: []assetbuild.Image{base}})

	return assetbuild.Result{"data": result}, err
}

func loadBakeMarks(ctx *assetbuild.Context, options BakeOptions, sources map[string]assetbuild.Ref) ([]bakedMark, error) {
	marks := make([]bakedMark, 0, len(options.Marks))
	images := make(map[string]assetbuild.Image, len(sources))

	for _, mark := range options.Marks {
		projector, err := mark.placement([2]float32{}, [2]float32{1, 1}).Project()
		if err != nil {
			return nil, err
		}

		if projector.Layers&options.Layers == 0 || projector.Tint[3] == 0 {
			continue
		}

		image, ok := images[mark.Image]
		if !ok {
			image, err = readBakeImage(ctx, sources[mark.Image])
			if err != nil {
				return nil, err
			}

			images[mark.Image] = image
		}

		marks = append(marks, bakedMark{image, projector})
	}

	slices.SortStableFunc(marks, func(a, b bakedMark) int { return cmp.Compare(a.projection.Order, b.projection.Order) })

	return marks, nil
}

func readBakeImage(ctx *assetbuild.Context, reference assetbuild.Ref) (assetbuild.Image, error) {
	set, err := readImages(ctx, reference)
	if err != nil {
		return assetbuild.Image{}, err
	}

	if len(set.Images) != 1 || set.Images[0].Atlas || set.Images[0].Usage != assetbuild.UsageAlbedo {
		return assetbuild.Image{}, fmt.Errorf("decal bake requires a standalone albedo: %w", decaldata.ErrProjector)
	}

	result := set.Images[0]
	result.Levels = result.Levels[:1]

	return result, nil
}
