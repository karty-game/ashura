//go:build !js

package lightmapbuild

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"math"

	"github.com/karty-game/ashura/internal/lightmapdata"
	"github.com/karty-game/ashura/internal/modeldata"
	"github.com/karty-game/ashura/internal/xatlas"
)

// PageOptions separates physical texel density from the maximum page size.
// Resolution in Options is unused; MaxPageSize defaults to1024, density to16,
// padding to4 and MaxPages to64. Existing RNM and transport options are reused.
type PageOptions struct {
	Options

	TexelsPerUnit float32
	MaxPageSize   uint32
	MaxPages      uint32
}

// Preparer retains one logical space's immutable visibility geometry and one
// embedded xatlas library. Receiver groups may be unrelated chunks of that space.
// All coordinates are world-space. Calls are sequential on one build worker.
type Preparer struct {
	atlas                      *xatlas.Session
	nodes, positions, surfaces []byte
	triangles                  []triangle
	patchUpperBound            uint64
}

// NewPreparer admits a complete visibility scene, builds its BVH and diffuse
// reflectance once, and owns the UV session for subsequent chunk jobs.
func NewPreparer(ctx context.Context, occluders modeldata.Document) (*Preparer, error) {
	if ctx == nil {
		return nil, errGeometry
	}

	if err := ctx.Err(); err != nil {
		return nil, err
	}

	if _, err := modeldata.Encode(occluders); err != nil {
		return nil, fmt.Errorf("lightmap visibility scene: %w", err)
	}

	triangles := collectTriangles(occluders.Parts)
	if len(triangles) == 0 {
		return nil, fmt.Errorf("visibility scene has no nondegenerate triangles: %w", errGeometry)
	}

	nodes, positions, err := buildSAHBVH(triangles)
	if err != nil {
		return nil, err
	}

	surfaces, err := buildSurfaces(triangles, occluders)
	if err != nil {
		return nil, err
	}

	if len(nodes)+len(positions)+len(surfaces) > lightmapdata.MaxTransportBytes {
		return nil, fmt.Errorf("visibility scene exceeds portable GPU buffer limit: %w", errGeometry)
	}

	atlas, err := xatlas.OpenSession(ctx)
	if err != nil {
		return nil, err
	}

	return &Preparer{atlas: atlas, nodes: nodes, positions: positions, surfaces: surfaces, triangles: triangles,
		patchUpperBound: transportPatchUpperBound(triangles, surfaces)}, nil
}

// Close releases the embedded UV session. No game runtime resources are created.
func (preparer *Preparer) Close() error {
	if preparer == nil {
		return nil
	}

	return preparer.atlas.Close()
}

// PreparePages is the one-shot convenience form. Reuse a Preparer when compiling
// several chunks to share visibility preparation and compiled UV code.
func PreparePages(ctx context.Context, receivers, occluders modeldata.Document, options PageOptions) ([]Result, error) {
	options, err := normalizePageOptions(options)
	if err != nil {
		return nil, err
	}

	preparer, err := NewPreparer(ctx, occluders)
	if err != nil {
		return nil, err
	}

	defer func() { _ = preparer.Close() }()

	return preparer.PreparePages(ctx, receivers, options)
}

// PreparePages charts only receivers, while visibility and bounce transport
// query the complete prepared scene. Each result binds its own remapped model.
func (preparer *Preparer) PreparePages(ctx context.Context, receivers modeldata.Document, options PageOptions) ([]Result, error) {
	if ctx == nil || preparer == nil || preparer.atlas == nil {
		return nil, errGeometry
	}

	if err := ctx.Err(); err != nil {
		return nil, err
	}

	options, err := normalizePageOptions(options)
	if err != nil {
		return nil, err
	}

	if _, err := modeldata.Encode(receivers); err != nil {
		return nil, fmt.Errorf("lightmap receiver geometry: %w", err)
	}

	return preparer.preparePages(ctx, receivers, options, preparer.preparePage)
}

type pageBuilder func(context.Context, modeldata.Document, xatlas.Page, PageOptions) (Result, error)

func (preparer *Preparer) preparePages(
	ctx context.Context,
	receivers modeldata.Document,
	options PageOptions,
	buildPage pageBuilder,
) ([]Result, error) {
	for {
		pages, err := preparer.atlas.GeneratePages(ctx, receivers.Parts, xatlas.PageOptions{
			TexelsPerUnit: options.TexelsPerUnit, Resolution: options.MaxPageSize, Padding: options.Padding, MaxPages: options.MaxPages,
		})
		if err != nil {
			return nil, err
		}

		pages, err = preparer.smallestSinglePage(ctx, receivers.Parts, pages, options)
		if err != nil {
			return nil, err
		}

		err = preparer.preflightPages(pages, options)

		var results []Result
		if err == nil {
			results, err = buildPageResults(ctx, receivers, pages, options, buildPage)
		}

		if !errors.Is(err, errTransportBudget) || pages[0].Width <= 16 {
			return results, err
		}
		// Density and geometry remain unchanged. Only the atlas capacity changes,
		// producing more independently bounded pages rather than a lower-resolution
		// solution or a larger storage limit.
		const reductionFactor = 2

		options.MaxPageSize = pages[0].Width / reductionFactor
	}
}

func buildPageResults(
	ctx context.Context,
	receivers modeldata.Document,
	pages []xatlas.Page,
	options PageOptions,
	buildPage pageBuilder,
) ([]Result, error) {
	results := make([]Result, len(pages))
	for index, page := range pages {
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		var err error

		results[index], err = buildPage(ctx, receivers, page, options)
		if err != nil {
			return nil, err
		}
	}

	return results, nil
}

// A maximum dimension is a memory cap, not a requirement to allocate mostly
// empty textures. Multi-page groups retain their configured full-size pages.
func (preparer *Preparer) smallestSinglePage(
	ctx context.Context,
	parts []modeldata.Part,
	pages []xatlas.Page,
	options PageOptions,
) ([]xatlas.Page, error) {
	if len(pages) != 1 {
		return pages, nil
	}

	const reductionFactor = 2

	for size := options.MaxPageSize / reductionFactor; size >= 16; size /= reductionFactor {
		if options.Padding*2+2 >= size {
			break
		}

		candidate, err := preparer.atlas.GeneratePages(ctx, parts, xatlas.PageOptions{
			TexelsPerUnit: options.TexelsPerUnit, Resolution: size, Padding: options.Padding, MaxPages: 1,
		})
		if err != nil {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}

			break
		}

		pages = candidate
	}

	return pages, nil
}

func normalizePageOptions(options PageOptions) (PageOptions, error) {
	if options.TexelsPerUnit == 0 {
		options.TexelsPerUnit = 16
	}

	if options.MaxPageSize == 0 {
		options.MaxPageSize = 1024
	}

	if options.MaxPages == 0 {
		options.MaxPages = 64
	}

	if options.Padding == 0 {
		options.Padding = 4
	}

	if math.IsNaN(float64(options.TexelsPerUnit)) || math.IsInf(float64(options.TexelsPerUnit), 0) ||
		options.TexelsPerUnit <= 0 || options.TexelsPerUnit > 4096 || options.MaxPageSize < 16 || options.MaxPageSize > 2048 ||
		options.MaxPageSize&(options.MaxPageSize-1) != 0 || options.Padding > 16 || options.Padding*2+2 >= options.MaxPageSize ||
		options.MaxPages > 64 {
		return options, errGeometry
	}

	normalized, err := normalizeTransportOptions(options.Options)
	options.Options = normalized

	return options, err
}

func (preparer *Preparer) preparePage(
	ctx context.Context,
	receivers modeldata.Document,
	page xatlas.Page,
	options PageOptions,
) (Result, error) {
	model, uvs, atlas := pageGeometry(receivers, page)

	doc := lightmapdata.Document{Width: page.Width, Height: page.Height}
	doc.UVs = uvs

	encodedModel, err := modeldata.Encode(model)
	if err != nil {
		return Result{}, err
	}

	doc.ModelSHA256 = sha256.Sum256(encodedModel)

	doc.Nodes, doc.Triangles = preparer.nodes, preparer.positions
	if options.RNM {
		doc.Surfaces = preparer.surfaces
	}

	var geometric []vec
	if options.PrecomputeTransport || options.PreserveGeometricNormals {
		geometric = make([]vec, int(page.Width)*int(page.Height))
	}

	doc.Receivers, doc.Padding, doc.Frames, err = buildReceivers(ctx, model.Parts, atlas, options.RNM, geometric)
	if err != nil {
		return Result{}, err
	}

	if options.PrecomputeTransport {
		if err := buildTransportMode(ctx, &doc, preparer.triangles, geometric, options.TransportSamples, true); err != nil {
			return Result{}, err
		}
	}

	encodedLighting, err := lightmapdata.Encode(doc)
	if err != nil {
		return Result{}, err
	}
	// Developer sidecars also travel through the portable128MiB asset reader;
	// reserve their UV streams and header as well as the GPU scene arrays.
	if len(encodedLighting) > lightmapdata.MaxTransportBytes {
		return Result{}, fmt.Errorf("prepared page sidecar needs%d bytes including UV/header: %w", len(encodedLighting), errTransportBudget)
	}

	covered := 0

	for offset := 12; offset < len(doc.Receivers); offset += 32 {
		if readFloat(doc.Receivers[offset:]) != 0 {
			covered++
		}
	}

	result := Result{Model: encodedModel, Lightmap: encodedLighting, Width: int(page.Width), Height: int(page.Height),
		Charts: int(page.Charts), CoveredTexels: covered}
	if options.PreserveGeometricNormals {
		result.GeometricNormals = encodeGeometricNormals(geometric)
	}

	return result, nil
}

// Pages embed only their actual material dependencies. Publication can externalize
// these textures by content identity without retaining unrelated source images.
func prunePageTextures(model *modeldata.Document) {
	original := model.Textures
	model.Textures = nil
	indices := make(map[int32]int32)

	for index := range model.Parts {
		material := &model.Parts[index].Material
		for _, field := range []*int32{&material.Albedo, &material.Normal, &material.AO} {
			if *field < 0 {
				continue
			}

			remapped, exists := indices[*field]
			if !exists {
				//nolint:gosec // The source document admits at most64 texture slots.
				remapped = int32(len(model.Textures))
				indices[*field] = remapped
				model.Textures = append(model.Textures, original[*field])
			}

			*field = remapped
		}
	}
}
