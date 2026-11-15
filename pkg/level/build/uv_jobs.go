//go:build !js

//nolint:gosec // Indexed triangle copies retain the bounded 32-byte vertex and uint32 index ABI.
package levelbuild

import (
	"encoding/binary"
	"fmt"

	"github.com/karty-game/karty-webgpu/internal/modeldata"
	"github.com/karty-game/karty-webgpu/pkg/assetbuild"
)

// Keep offline preparation work independent of runtime chunk size. Each job
// retains prepare's existing two-minute deadline and full-space visibility.
const uvJobTriangles = 32768

func (plugin *uvPlugin) planUVJobs(
	_ *assetbuild.Context,
	plan *assetbuild.Plan,
	identity string,
	chunk chunkRecord,
) (assetbuild.Ref, error) {
	roster, err := addReceiverJobs(plan, identity, chunk)
	if err != nil {
		return assetbuild.Ref{}, err
	}

	err = plan.Expand(assetbuild.Expansion{
		ID: "level/receiver-jobs/discover/" + identity + "/" + chunk.Description.ID, Inputs: roster,
		Run: func(ctx *assetbuild.Context, child *assetbuild.Plan) error {
			var jobs []assetbuild.Artifact
			if err := readRecord(ctx, roster[0], &jobs); err != nil {
				return err
			}

			return plugin.discoverUVJobs(child, identity, chunk, jobs)
		},
	})

	return assetbuild.Ref{Action: "level/prepare-index/" + identity + "/" + chunk.Description.ID, Slot: slot}, err
}

func addReceiverJobs(plan *assetbuild.Plan, identity string, chunk chunkRecord) ([]assetbuild.Ref, error) {
	return plan.Add(assetbuild.Action{
		ID:        "level/receiver-jobs/" + identity + "/" + chunk.Description.ID,
		Processor: "level-receiver-jobs", Revision: "triangle-bounds@1", Artifacts: []assetbuild.Artifact{chunk.Geometry},
		Options: uvJobTriangles, Outputs: []assetbuild.Slot{{Name: slot, Schema: "karty.level-receiver-jobs@1"}},
		Run: func(ctx *assetbuild.Context) (assetbuild.Result, error) {
			return splitReceiverJobs(ctx, chunk.Geometry)
		},
	})
}

func splitReceiverJobs(ctx *assetbuild.Context, geometry assetbuild.Artifact) (assetbuild.Result, error) {
	data, err := ctx.ReadArtifact(geometry)
	if err != nil {
		return nil, err
	}

	model, err := modeldata.Decode(data)
	if err != nil {
		return nil, err
	}

	var output []assetbuild.Artifact

	for _, model := range receiverJobs(model, uvJobTriangles) {
		data, err := modeldata.Encode(model)
		if err != nil {
			return nil, err
		}

		artifact, err := ctx.StoreArtifact(data)
		if err != nil {
			return nil, err
		}

		output = append(output, artifact)
	}

	return result(output)
}

func (plugin *uvPlugin) discoverUVJobs(plan *assetbuild.Plan, identity string, chunk chunkRecord, jobs []assetbuild.Artifact) error {
	var refs []assetbuild.Ref

	for index, geometry := range jobs {
		job := chunk
		job.Geometry = geometry

		artifacts := make([]assetbuild.Artifact, 0, 2+len(job.SSBump))

		artifacts = append(artifacts, job.Geometry, job.Occluders)
		for _, ref := range job.SSBump {
			artifacts = append(artifacts, ref)
		}

		outputs, err := plan.Add(assetbuild.Action{
			Parallel:  plugin.options.Production,
			ID:        fmt.Sprintf("level/prepare/%s/%s/work/%d", identity, chunk.Description.ID, index),
			Processor: "level-uv", Revision: "bounded-receivers@6-wasi1", Artifacts: artifacts,
			Options: struct {
				UV     UVOptions
				SSBump map[string]assetbuild.Artifact
			}{plugin.options, job.SSBump},
			Outputs: []assetbuild.Slot{{Name: slot, Schema: uvSchema}},
			Run:     func(ctx *assetbuild.Context) (assetbuild.Result, error) { return plugin.prepare(ctx, job) },
		})
		if err != nil {
			return err
		}

		refs = append(refs, outputs[0])
	}

	_, err := plan.Add(assetbuild.Action{
		ID:        "level/prepare-index/" + identity + "/" + chunk.Description.ID,
		Processor: "level-uv-jobs", Revision: "ordered-pages@1", Inputs: refs,
		Outputs: []assetbuild.Slot{{Name: slot, Schema: uvSchema}},
		Run: func(ctx *assetbuild.Context) (assetbuild.Result, error) {
			var output []pageRecord

			for _, ref := range refs {
				var pages []pageRecord
				if err := readRecord(ctx, ref, &pages); err != nil {
					return nil, err
				}

				output = append(output, pages...)
			}

			return result(output)
		},
	})
	if err != nil {
		return err
	}

	return nil
}

// receiverJobs copies triangles in source order, retaining material names,
// positions, normals and UV0 exactly. No new spatial boundary is introduced.
func receiverJobs(model modeldata.Document, maximum int) []modeldata.Document {
	var jobs []modeldata.Document

	job := modeldata.Document{Source: model.Source, Textures: model.Textures}
	count := 0

	for _, source := range model.Parts {
		remap := map[uint32]uint32{}
		part := source

		part.Vertices, part.Indices = nil, nil
		for offset := 0; offset < len(source.Indices); offset += 12 {
			if count == maximum {
				if len(part.Indices) > 0 {
					job.Parts = append(job.Parts, part)
				}

				jobs = append(jobs, job)
				job = modeldata.Document{Source: model.Source, Textures: model.Textures}
				count = 0
				part.Vertices, part.Indices = nil, nil

				clear(remap)
			}

			for corner := range 3 {
				original := binary.LittleEndian.Uint32(source.Indices[offset+corner*4:])

				index, exists := remap[original]
				if !exists {
					index = uint32(len(part.Vertices) / 32)
					remap[original] = index
					part.Vertices = append(part.Vertices, source.Vertices[int(original)*32:int(original+1)*32]...)
				}

				part.Indices = binary.LittleEndian.AppendUint32(part.Indices, index)
			}

			count++
		}

		if len(part.Indices) > 0 {
			job.Parts = append(job.Parts, part)
		}
	}

	if count > 0 {
		jobs = append(jobs, job)
	}

	return jobs
}
