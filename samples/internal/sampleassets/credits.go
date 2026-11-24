//go:build !js

package sampleassets

import "github.com/karty-game/ashura/pkg/assetbuild"

// Credits publishes a sample's asset notices alongside its web resources.
func Credits(source, filename string) assetbuild.Pipeline {
	return assetbuild.Pipeline{
		Name: "credits", Directory: "public",
		Output: []assetbuild.Plugin{credits{Source: source, Filename: filename}},
	}
}

type credits struct {
	Source   string
	Filename string
}

func (credits) Name() string { return "sample-credits" }

func (notice credits) Plan(plan *assetbuild.Plan) error {
	const slot = "files"

	refs, err := plan.Add(assetbuild.Action{
		ID: notice.Name(), Processor: notice.Name(), Revision: "copy@1", Options: notice,
		Outputs: []assetbuild.Slot{{Name: slot, Schema: "karty.files@1"}},
		Run: func(ctx *assetbuild.Context) (assetbuild.Result, error) {
			data, err := ctx.ReadFile(notice.Source)
			if err != nil {
				return nil, err
			}

			bundle, err := assetbuild.EncodeBundle(assetbuild.Bundle{notice.Filename: data})
			if err != nil {
				return nil, err
			}

			return assetbuild.Result{slot: bundle}, nil
		},
	})
	if err != nil {
		return err
	}

	plan.Publish(refs[0])

	return nil
}
