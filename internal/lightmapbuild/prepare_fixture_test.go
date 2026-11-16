//go:build !js

package lightmapbuild

import (
	"context"
	"crypto/sha256"

	"github.com/karty-game/karty-webgpu/internal/lightmapdata"
	"github.com/karty-game/karty-webgpu/internal/modeldata"
	"github.com/karty-game/karty-webgpu/internal/xatlas"
)

// prepareFixture assembles one complete model so direct geometry, receiver and
// transport tests can exercise the shared preparation algorithms together.
func prepareFixture(ctx context.Context, data []byte, options Options) (Result, error) {
	options, err := normalizeTransportOptions(options)
	if err != nil {
		return Result{}, err
	}

	model, err := decodeGeometry(data)
	if err != nil {
		return Result{}, err
	}

	session, err := xatlas.OpenSession(ctx)
	if err != nil {
		return Result{}, err
	}
	defer func() { _ = session.Close() }()

	atlas, err := session.Generate(ctx, model.Parts, options.Resolution, options.Padding)
	if err != nil {
		return Result{}, err
	}

	doc := lightmapdata.Document{Width: atlas.Width, Height: atlas.Height, UVs: make([][]byte, len(model.Parts))}
	doc.UVs = remapModel(&model, atlas)

	remapped, err := modeldata.Encode(model)
	if err != nil {
		return Result{}, err
	}

	doc.ModelSHA256 = sha256.Sum256(remapped)

	var geometricNormals []byte
	if err := prepareLighting(ctx, &doc, model, atlas, options, &geometricNormals); err != nil {
		return Result{}, err
	}

	output, err := lightmapdata.Encode(doc)
	if err != nil {
		return Result{}, err
	}

	covered := 0

	for offset := 12; offset < len(doc.Receivers); offset += 32 {
		if readFloat(doc.Receivers[offset:]) != 0 {
			covered++
		}
	}

	return Result{
		Model:            remapped,
		Lightmap:         output,
		GeometricNormals: geometricNormals,
		Width:            int(atlas.Width),
		Height:           int(atlas.Height),
		Charts:           int(atlas.Charts),
		CoveredTexels:    covered,
	}, nil
}
