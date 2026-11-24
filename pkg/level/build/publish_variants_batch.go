//go:build !js

package levelbuild

import (
	"fmt"

	"github.com/karty-game/ashura/pkg/assetbuild"
)

type nativeVariantState struct {
	resource assetbuild.Resource
	options  PublishOptions
	refs     []assetbuild.Ref
}

func nativeVariantBatch(profiles []PublishOptions) bool {
	for _, profile := range profiles {
		if profile.NativeTextures == nil || !profile.NativeTextures.NativeAlbedo || profile.AlbedoETC1S == nil {
			return false
		}
	}

	return true
}

// Plan each shared preparation stage for every profile before joining it. This
// leaves independent RGB/RNM maps, relief and albedo encoders in one ready graph
// instead of finishing all encoders for one device profile before the next.
func planNativeVariants(plan *assetbuild.Plan, profiles []PublishOptions) error {
	return (stage{name: "publish-variants-" + profiles[0].Variant, schema: bakedSchema, run: func(plan *assetbuild.Plan, resource assetbuild.Resource) error {
		return plan.Expand(assetbuild.Expansion{
			ID: "level/publish-variants/" + resource.ID + "/" + profiles[0].Variant, Inputs: []assetbuild.Ref{resource.Ref},
			Run: func(ctx *assetbuild.Context, child *assetbuild.Plan) error {
				return discoverVariantPages(ctx, child, resource, profiles)
			},
		})
	}}).Plan(
		plan,
	)
}

func discoverVariantPages(ctx *assetbuild.Context, plan *assetbuild.Plan, resource assetbuild.Resource, profiles []PublishOptions) error {
	var record partitioned
	if err := readRecord(ctx, resource.Ref, &record); err != nil {
		return err
	}

	states := make([]nativeVariantState, len(profiles))
	for index, profile := range profiles {
		current := resource
		current.ID += "/" + profile.Variant

		refs, err := planPublishedPages(plan, current, record, profile)
		if err != nil {
			return err
		}

		states[index] = nativeVariantState{resource: current, options: profile, refs: refs}
	}

	return expandVariantStage(plan, resource, record, states, "atlases", discoverVariantAtlases)
}

func discoverVariantAtlases(ctx *assetbuild.Context, plan *assetbuild.Plan, resource assetbuild.Resource,
	record partitioned, states []nativeVariantState) error {
	for index := range states {
		state := &states[index]

		refs, err := planConsolidatedChunks(ctx, plan, state.resource, record, state.refs, state.options)
		if err != nil {
			return err
		}

		state.refs = refs
	}

	return expandVariantStage(plan, resource, record, states, "normalize", discoverVariantNormalized)
}

func discoverVariantNormalized(ctx *assetbuild.Context, plan *assetbuild.Plan, resource assetbuild.Resource,
	record partitioned, states []nativeVariantState) error {
	for index := range states {
		state := &states[index]

		refs, err := planNormalizedLDR(ctx, plan, state.resource, state.refs, state.options, *state.options.NativeTextures)
		if err != nil {
			return err
		}

		state.refs = refs
	}

	return expandVariantStage(plan, resource, record, states, "codecs", discoverVariantCodecs)
}

type nativeVariantCodecs struct {
	pages             [][]publishedPage
	textures, albedos map[string]assetbuild.Ref
	dependencies      []assetbuild.Ref
}

func discoverVariantCodecs(ctx *assetbuild.Context, plan *assetbuild.Plan, resource assetbuild.Resource,
	record partitioned, states []nativeVariantState) error {
	codecs := make([]nativeVariantCodecs, len(states))
	for index, state := range states {
		current, err := planVariantCodecs(ctx, plan, state)
		if err != nil {
			return err
		}

		codecs[index] = current
	}
	// Indices are declared after every encoder, so they cannot insert a serial
	// action boundary into the ready region containing another profile's codecs.
	for index := range states {
		refs, err := planVariantIndices(plan, states[index], codecs[index])
		if err != nil {
			return err
		}

		states[index].refs = refs
	}

	return expandVariantStage(plan, resource, record, states, "publish", discoverVariantManifests)
}

func planVariantCodecs(ctx *assetbuild.Context, plan *assetbuild.Plan, state nativeVariantState) (nativeVariantCodecs, error) {
	profile := *state.options.NativeTextures

	inputs, err := collectNativeBlocks(ctx, state.refs)
	if err != nil {
		return nativeVariantCodecs{}, err
	}

	textures, dependencies, err := nativeTextureActions(plan, state.resource.ID, inputs, profile, state.options.sharedCodecs)
	if err != nil {
		return nativeVariantCodecs{}, err
	}

	encoder := *state.options.AlbedoETC1S
	if encoder.Quality < 1 || encoder.Quality > 255 {
		return nativeVariantCodecs{}, fmt.Errorf("invalid ETC1S encoder options: %w", ErrInvalid)
	}

	_, albedos, err := collectAlbedos(ctx, state.refs)
	if err != nil {
		return nativeVariantCodecs{}, err
	}

	prepared, _, err := encodedAlbedoActions(plan, state.resource.ID, albedos, encoder, state.options.sharedCodecs, true)
	if err != nil {
		return nativeVariantCodecs{}, err
	}

	encoded, refs, err := nativeVariantAlbedos(plan, state, prepared)
	if err != nil {
		return nativeVariantCodecs{}, err
	}

	return nativeVariantCodecs{pages: inputs.pages, textures: textures, albedos: encoded, dependencies: append(dependencies, refs...)}, nil
}

func discoverVariantManifests(ctx *assetbuild.Context, plan *assetbuild.Plan, _ assetbuild.Resource,
	record partitioned, states []nativeVariantState) error {
	for _, state := range states {
		if err := publishLevel(ctx, plan, state.resource, record, state.refs, state.options); err != nil {
			return err
		}
	}

	return nil
}

func expandVariantStage(plan *assetbuild.Plan, resource assetbuild.Resource, record partitioned,
	states []nativeVariantState, phase string,
	run func(*assetbuild.Context, *assetbuild.Plan, assetbuild.Resource, partitioned, []nativeVariantState) error) error {
	inputs := []assetbuild.Ref{resource.Ref}
	for _, state := range states {
		inputs = append(inputs, state.refs...)
	}

	return plan.Expand(assetbuild.Expansion{
		ID: "level/variants/" + phase + "/" + resource.ID + "/" + states[0].options.Variant, Inputs: inputs,
		Run: func(ctx *assetbuild.Context, child *assetbuild.Plan) error {
			return run(ctx, child, resource, record, states)
		},
	})
}
