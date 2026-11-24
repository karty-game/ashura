//go:build !js

package levelbuild

import (
	"encoding/hex"
	"fmt"
	"math"

	"github.com/karty-game/ashura/pkg/assetbuild"
	bake "github.com/karty-game/ashura/pkg/lightmap/bake"
)

const (
	productionSupportSchema = "karty.lightmapper-surface-support@4"
	supportPartMagic        = "KRTYSG02"
	supportPartHeader       = 24
	supportEdgeBytes        = 4
	supportRowBytes         = 8
	supportArtifactBytes    = 128 << 20
)

type productionSupport struct {
	Schema       string                `json:"schema"`
	GeometryHash string                `json:"geometryHash"`
	ReceiverHash string                `json:"receiverHash"`
	Count        uint64                `json:"count"`
	Rows         uint64                `json:"rows"`
	Parts        []assetbuild.Artifact `json:"parts"`
	Diagnostics  assetbuild.Artifact   `json:"diagnostics"`
}

type productionSupportOptions struct {
	Enabled bool                    `json:"enabled"`
	Radius  float32                 `json:"radius"`
	Limits  bake.SurfaceEdgeOptions `json:"limits"`
}

// planProductionSupport caches visibility-tested surface adjacency independently
// of lighting, sampling, backend selection and the resolved lightmap values.
func (plugin *productionPlugin) planProductionSupport(
	plan *assetbuild.Plan,
	identity string,
	chunk chunkRecord,
	geometryRef assetbuild.Ref,
) (assetbuild.Ref, error) {
	options := canonicalSupportOptions(plugin.options)

	artifacts := make([]assetbuild.Artifact, 0, 2*len(chunk.Pages))
	for _, page := range chunk.Pages {
		artifacts = append(artifacts, page.Receivers, page.Geometry)
	}

	refs, err := plan.Add(assetbuild.Action{
		Parallel: true, Exclusive: plugin.supportResource,
		ID:        "level/production/support/" + identity + "/" + chunk.Description.ID,
		Processor: "lightmapper-surface-support", Revision: "geometry-page-inputs-dense-rows@4",
		Inputs: []assetbuild.Ref{geometryRef}, Artifacts: artifacts, Options: options,
		Outputs: []assetbuild.Slot{{Name: slot, Schema: productionSupportSchema}},
		Run: func(ctx *assetbuild.Context) (assetbuild.Result, error) {
			scene, err := plugin.readScene(ctx, geometryRef)
			if err != nil {
				return nil, err
			}

			receivers, err := plugin.readSupportReceivers(ctx, chunk.Pages, scene)
			if err != nil {
				return nil, err
			}

			var report bake.SurfaceEdgeReport
			if options.Enabled && len(receivers) > 0 {
				report, err = bake.BuildSurfaceEdgesReportContext(ctx, scene, receivers, options.Radius, options.Limits)
				if err != nil {
					return nil, err
				}
			}

			identity, err := productionSupportIdentity(scene.GeometryHash(), chunk.Pages)
			if err != nil {
				return nil, err
			}

			return storeProductionSupport(ctx, scene.GeometryHash(), identity, receivers, report)
		},
	})
	if err != nil {
		return assetbuild.Ref{}, err
	}

	return refs[0], nil
}

func (plugin *productionPlugin) readSupportReceivers(
	ctx *assetbuild.Context,
	pages []pageRecord,
	scene *bake.PreparedScene,
) ([]bake.Receiver, error) {
	sets := make([]bake.ReceiverSet, len(pages))
	reader := productionPageReader{ctx: ctx, plugin: plugin}

	var count int

	for index, page := range pages {
		set, err := plugin.readProductionReceiverPage(ctx, scene, page, reader.artifact)
		if err != nil {
			return nil, err
		}

		if len(set.Receivers) > math.MaxInt-count {
			return nil, ErrInvalid
		}

		sets[index] = set
		count += len(set.Receivers)
	}

	receivers := make([]bake.Receiver, 0, count)
	identities := make(map[uint64]bool, count)

	for _, set := range sets {
		for _, receiver := range set.Receivers {
			if identities[receiver.ID] {
				return nil, fmt.Errorf("support receiver identity collision: %w", ErrInvalid)
			}

			identities[receiver.ID] = true
		}

		receivers = append(receivers, set.Receivers...)
	}

	plugin.pageInputs.rememberGroup(productionReceiverGroupKey(scene, pages), receivers)

	return receivers, nil
}

func storeProductionSupport(
	ctx *assetbuild.Context,
	geometryHash, receiverIdentity string,
	receivers []bake.Receiver,
	report bake.SurfaceEdgeReport,
) (assetbuild.Result, error) {
	adjacency, err := indexSupportRows(report, receivers)
	if err != nil {
		return nil, err
	}

	index := productionSupport{
		Schema: productionSupportSchema, GeometryHash: geometryHash, ReceiverHash: receiverIdentity,
		Count: uint64(len(adjacency.Targets)), Rows: uint64(len(adjacency.Owners)),
	}

	diagnostics, err := encodeSupportDiagnostics(report)
	if err != nil {
		return nil, err
	}

	index.Diagnostics, err = ctx.StoreArtifact(diagnostics)
	if err != nil {
		return nil, err
	}

	for start := 0; start < len(adjacency.Owners); {
		end := supportPartEnd(adjacency, start)
		if end == start {
			return nil, ErrInvalid
		}

		data, err := encodeSupportPart(adjacency, start, end)
		if err != nil {
			return nil, err
		}

		artifact, err := ctx.StoreArtifact(data)
		if err != nil {
			return nil, err
		}

		index.Parts = append(index.Parts, artifact)
		start = end
	}

	return result(index)
}

type productionReconstructionSupport struct {
	Prepared            *bake.ReconstructionSupport
	IsolatedReceiverIDs []uint64
}

func readProductionSupportReport(
	ctx *assetbuild.Context,
	ref assetbuild.Ref,
	geometryHash string,
	receivers []bake.Receiver,
	pages []pageRecord,
	maximumEdges uint64,
) (productionReconstructionSupport, error) {
	var result productionReconstructionSupport

	var report bake.SurfaceEdgeReport

	index, err := readProductionSupportManifest(ctx, ref, geometryHash, len(receivers), pages, maximumEdges)
	if err != nil {
		return result, err
	}

	data, err := ctx.ReadArtifact(index.Diagnostics)
	if err != nil {
		return result, err
	}

	report, err = decodeSupportDiagnostics(data, uint64(len(receivers)))
	if err != nil {
		return result, err
	}

	owners, err := supportReportOwners(report, receivers)
	if err != nil {
		return result, err
	}

	//nolint:gosec // G115: the manifest bounds counts by MaxInt and MaxUint32 before allocation.
	adjacency := bake.ReconstructionAdjacency{
		Owners: make([]uint32, 0, int(index.Rows)), Offsets: make([]uint32, 1, int(index.Rows)+1),
		Targets: make([]uint32, 0, int(index.Count)),
	}
	for index, receiver := range receivers {
		if owners[receiver.ID] == supportOwnerIsolated {
			adjacency.Isolated = append(adjacency.Isolated, uint32(index))
		}
	}

	for _, artifact := range index.Parts {
		data, err := ctx.ReadArtifact(artifact)
		if err != nil {
			return result, err
		}

		adjacency, err = appendSupportPart(adjacency, data)
		if err != nil {
			return result, err
		}
	}

	if uint64(len(adjacency.Targets)) != index.Count || uint64(len(adjacency.Owners)) != index.Rows {
		return result, fmt.Errorf("surface support incomplete graph: %w", ErrInvalid)
	}

	result.Prepared, err = bake.PrepareReconstructionSupport(receivers, adjacency)
	if err != nil {
		return productionReconstructionSupport{}, err
	}

	result.IsolatedReceiverIDs = report.IsolatedReceiverIDs

	return result, nil
}

func readProductionSupportManifest(
	ctx *assetbuild.Context,
	ref assetbuild.Ref,
	geometryHash string,
	receiverCount int,
	pages []pageRecord,
	maximumEdges uint64,
) (productionSupport, error) {
	var index productionSupport
	if err := readRecord(ctx, ref, &index); err != nil {
		return productionSupport{}, err
	}

	identity, err := productionSupportIdentity(geometryHash, pages)
	if err != nil {
		return productionSupport{}, err
	}

	if err := validateSupportIndexIdentity(index, geometryHash, identity, receiverCount); err != nil {
		return productionSupport{}, err
	}

	if index.Count > maximumEdges || index.Count > math.MaxUint32 || index.Rows > math.MaxUint32 ||
		index.Count > uint64(math.MaxInt)/supportEdgeBytes || index.Rows >= uint64(math.MaxInt)/supportRowBytes {
		return productionSupport{}, ErrInvalid
	}

	return index, nil
}

func validateSupportIndexIdentity(index productionSupport, geometryHash, receiverHash string, receiverCount int) error {
	decodedHash, err := hex.DecodeString(geometryHash)
	if err != nil || len(decodedHash) != 32 || index.Schema != productionSupportSchema ||
		index.GeometryHash != geometryHash || index.ReceiverHash != receiverHash {
		return fmt.Errorf("surface support geometry/ordered receiver identity mismatch: %w", ErrInvalid)
	}

	if receiverCount < 0 {
		return ErrInvalid
	}

	count := uint64(receiverCount)
	if count > math.MaxUint32 || index.Count > math.MaxUint32 || index.Rows > math.MaxUint32 || (count == 0 && index.Count != 0) ||
		(count > 0 && index.Count > count*(count-1)) || index.Rows > count || index.Rows > index.Count ||
		(index.Count == 0) != (index.Rows == 0) || uint64(len(index.Parts)) > index.Rows {
		return fmt.Errorf("surface support impossible graph count: %w", ErrInvalid)
	}

	return validateSupportParts(index)
}

func validateSupportParts(index productionSupport) error {
	diagnosticHash, diagnosticErr := hex.DecodeString(index.Diagnostics.SHA256)
	if diagnosticErr != nil || len(diagnosticHash) != 32 || index.Diagnostics.Size < supportDiagnosticHeader ||
		index.Diagnostics.Size > supportArtifactBytes {
		return fmt.Errorf("surface support invalid diagnostics artifact: %w", ErrInvalid)
	}

	var total uint64

	for _, artifact := range index.Parts {
		if artifact.Size < supportPartHeader+supportRowBytes+supportEdgeBytes || artifact.Size > supportArtifactBytes ||
			(artifact.Size-supportPartHeader)%supportEdgeBytes != 0 {
			return fmt.Errorf("surface support invalid part size: %w", ErrInvalid)
		}

		total += uint64(artifact.Size - supportPartHeader)
	}

	if total != index.Count*supportEdgeBytes+index.Rows*supportRowBytes {
		return fmt.Errorf("surface support declared payload count mismatch: %w", ErrInvalid)
	}

	return nil
}

func validSupportEdge(edge bake.ReconstructionEdge) bool {
	return edge.From != edge.To && edge.Weight > 0 && !math.IsNaN(float64(edge.Weight)) && !math.IsInf(float64(edge.Weight), 0)
}

func canonicalSupportOptions(options ProductionOptions) productionSupportOptions {
	result := productionSupportOptions{Limits: options.SurfaceEdges}

	result.Limits.Support = nil
	if options.Reconstruction == nil || options.Reconstruction.Strength == 0 {
		return result
	}

	geometry := *options.Reconstruction
	geometry.NoiseScale, geometry.Strength, geometry.MaximumHalo = 1, 1, 1
	result.Enabled, result.Radius, result.Limits.Support = true, geometry.Radius, &geometry

	return result
}
