//go:build !js

//nolint:gosec // Model limits bound old/new triangle indices and receiver identity words.
package levelbuild

import (
	"crypto/sha256"
	"encoding/binary"
	"math"

	"github.com/karty-game/ashura/internal/leveldata"
	"github.com/karty-game/ashura/internal/lightmapdata"
	"github.com/karty-game/ashura/internal/meshopt"
	"github.com/karty-game/ashura/internal/modeldata"
	"github.com/karty-game/ashura/pkg/assetbuild"
	bake "github.com/karty-game/ashura/pkg/lightmap/bake"
)

func optimizeUVPage(ctx *assetbuild.Context, page pageRecord) (pageRecord, error) {
	data, err := ctx.ReadArtifact(page.Geometry)
	if err != nil {
		return pageRecord{}, err
	}

	model, err := modeldata.Decode(data)
	if err != nil {
		return pageRecord{}, err
	}

	uvs, legacy, err := pageGeometryUVs(ctx, page, sha256.Sum256(data))
	if err != nil {
		return pageRecord{}, err
	}

	model, uvs, triangles, err := meshopt.OptimizeUV(ctx, model, uvs)
	if err != nil {
		return pageRecord{}, err
	}

	data, err = modeldata.Encode(model)
	if err != nil {
		return pageRecord{}, err
	}

	digest := sha256.Sum256(data)

	page.Geometry, err = ctx.StoreArtifact(data)
	if err != nil {
		return pageRecord{}, err
	}

	if page.UVLayout.Size > 0 {
		data, err = leveldata.EncodeUV(digest, uvs)
		if err != nil {
			return pageRecord{}, err
		}

		page.UVLayout, err = ctx.StoreArtifact(data)
		if err != nil {
			return pageRecord{}, err
		}
	}

	if legacy != nil {
		legacy.ModelSHA256 = digest
		legacy.UVs = uvs

		data, err = lightmapdata.Encode(*legacy)
		if err != nil {
			return pageRecord{}, err
		}

		page.BakeInputs, err = ctx.StoreArtifact(data)
		if err != nil {
			return pageRecord{}, err
		}
	}

	if page.Receivers.Size > 0 {
		page.Receivers, err = optimizeReceiverReferences(ctx, page, triangles, digest)
		if err != nil {
			return pageRecord{}, err
		}
	}

	return page, nil
}

func pageGeometryUVs(ctx *assetbuild.Context, page pageRecord, digest [32]byte) ([][]byte, *lightmapdata.Document, error) {
	var (
		legacy *lightmapdata.Document
		uvs    [][]byte
	)

	if page.BakeInputs.Size > 0 {
		data, err := ctx.ReadArtifact(page.BakeInputs)
		if err != nil {
			return nil, nil, err
		}

		legacy, err = lightmapdata.Decode(data)
		if err != nil {
			return nil, nil, err
		}

		if legacy.ModelSHA256 != digest {
			return nil, nil, ErrInvalid
		}

		uvs = legacy.UVs
	}

	if page.UVLayout.Size > 0 {
		data, err := ctx.ReadArtifact(page.UVLayout)
		if err != nil {
			return nil, nil, err
		}

		layout, err := leveldata.AdmitUVOwned(data)
		if err != nil {
			return nil, nil, err
		}

		if layout.ModelDigest() != digest {
			return nil, nil, ErrInvalid
		}

		uvs = make([][]byte, layout.PartCount())
		for index := range uvs {
			uvs[index] = layout.PartUV(index)
		}
	}

	if len(uvs) == 0 {
		return nil, nil, ErrInvalid
	}

	return uvs, legacy, nil
}

func optimizeReceiverReferences(
	ctx *assetbuild.Context,
	page pageRecord,
	triangles []uint32,
	digest [32]byte,
) (assetbuild.Artifact, error) {
	data, err := ctx.ReadArtifact(page.Receivers)
	if err != nil {
		return assetbuild.Artifact{}, err
	}

	set, err := bake.DecodeReceiverSet(data)
	if err != nil {
		return assetbuild.Artifact{}, err
	}

	if set.Width != page.Width || set.Height != page.Height {
		return assetbuild.Artifact{}, ErrInvalid
	}

	for index := range set.Receivers {
		receiver := &set.Receivers[index]
		for piece := range receiver.Pieces {
			original := receiver.Pieces[piece].Triangle
			if uint64(original) >= uint64(len(triangles)) {
				return assetbuild.Artifact{}, ErrInvalid
			}

			receiver.Pieces[piece].Triangle = triangles[original]
		}

		for _, patch := range receiver.Patches {
			if patch.Triangle != math.MaxUint32 {
				return assetbuild.Artifact{}, ErrInvalid
			}
		}

		var identity [40]byte
		copy(identity[:32], digest[:])
		binary.LittleEndian.PutUint32(identity[32:], receiver.Pixel)
		binary.LittleEndian.PutUint32(identity[36:], uint32(receiver.Side))
		sum := sha256.Sum256(identity[:])
		receiver.ID = binary.LittleEndian.Uint64(sum[:8])
	}

	data, err = bake.EncodeReceiverSet(set)
	if err != nil {
		return assetbuild.Artifact{}, err
	}

	return ctx.StoreArtifact(data)
}
