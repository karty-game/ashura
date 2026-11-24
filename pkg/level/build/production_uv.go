//go:build !js

package levelbuild

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"time"

	"github.com/karty-game/ashura/internal/leveldata"
	preparation "github.com/karty-game/ashura/internal/lightmapbuild"
	"github.com/karty-game/ashura/internal/modeldata"
	"github.com/karty-game/ashura/pkg/assetbuild"
	bake "github.com/karty-game/ashura/pkg/lightmap/bake"
)

func (plugin *uvPlugin) prepareProductionUV(ctx *assetbuild.Context, chunk chunkRecord) (assetbuild.Result, error) {
	bounded, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()

	preparer, err := plugin.productionPreparer(bounded)
	if err != nil {
		return nil, err
	}

	data, err := ctx.ReadArtifact(chunk.Geometry)
	if err != nil {
		return nil, err
	}

	model, err := modeldata.Decode(data)
	if err != nil {
		return nil, err
	}

	model, bumps := independentReceiverSides(model, chunk.SSBump)

	output := make([]pageRecord, 0)

	err = preparer.PrepareEach(
		bounded,
		model,
		preparation.PageOptions{
			TexelsPerUnit: plugin.options.TexelsPerUnit,
			MaxPageSize:   plugin.options.MaxPageSize,
			RNM:           true, Padding: plugin.options.Padding,
		},
		func(page preparation.UVPage) error {
			record, storeErr := storeProductionUVPage(ctx, page, bumps)
			if storeErr != nil {
				return storeErr
			}

			plugin.receiverCache.seed(record, bake.ReceiverSet{
				Schema: "karty.lightmapper-receivers@2", Width: page.Width, Height: page.Height,
				Receivers: page.Receivers, Gutters: page.GutterOwners,
			})

			output = append(output, record)

			return nil
		},
	)
	if err != nil {
		return nil, err
	}

	return result(output)
}

// Initialization owns the only mutable setup. Each preparation request starts
// its own WASM instance and owns its geometry, buffers and page callbacks.
func (plugin *uvPlugin) productionPreparer(ctx context.Context) (*preparation.UVPreparer, error) {
	plugin.productionMu.Lock()
	defer plugin.productionMu.Unlock()

	if plugin.production == nil {
		preparer, err := preparation.NewUVPreparer(ctx)
		if err != nil {
			return nil, err
		}

		plugin.production = preparer
	}

	return plugin.production, nil
}

// The build coordinator joins all active actions before running cleanup.
func (plugin *uvPlugin) closeProductionPreparer() error {
	plugin.productionMu.Lock()
	defer plugin.productionMu.Unlock()

	if plugin.production == nil {
		return nil
	}

	err := plugin.production.Close()
	plugin.production = nil

	return err
}

//nolint:gosec // Side is an exact signed int32 bit pattern in the receiver identity preimage.
func storeProductionUVPage(ctx *assetbuild.Context, page preparation.UVPage, bumps map[string]assetbuild.Artifact) (pageRecord, error) {
	record := pageRecord{Width: page.Width, Height: page.Height, SSBump: bumps}

	var err error

	record.Geometry, err = ctx.StoreArtifact(page.Model)
	if err != nil {
		return record, err
	}

	uv, err := leveldata.EncodeUV(sha256.Sum256(page.Model), page.UVs)
	if err != nil {
		return record, err
	}

	record.UVLayout, err = ctx.StoreArtifact(uv)
	if err != nil {
		return record, err
	}

	// Coverage validation already clipped these exact model triangles. Reuse
	// those footprints instead of repeating clipping and transport preparation.
	receivers := page.Receivers

	modelDigest := sha256.Sum256(page.Model)

	for index := range receivers {
		var identity [40]byte
		copy(identity[:32], modelDigest[:])
		binary.LittleEndian.PutUint32(identity[32:], receivers[index].Pixel)
		binary.LittleEndian.PutUint32(identity[36:], uint32(receivers[index].Side))
		digest := sha256.Sum256(identity[:])
		receivers[index].ID = binary.LittleEndian.Uint64(digest[:8])

		chart := page.ChartOwners[receivers[index].Pixel]
		if chart < 0 {
			return record, ErrInvalid
		}

		receivers[index].Chart = uint32(chart)
	}

	payload, err := bake.EncodeReceiverSet(
		bake.ReceiverSet{Width: page.Width, Height: page.Height, Receivers: receivers, Gutters: page.GutterOwners},
	)
	if err != nil {
		return record, err
	}

	record.Receivers, err = ctx.StoreArtifact(payload)
	if err != nil {
		return record, err
	}
	// Preserve the stable record protocol; large data only appears in artifacts.
	if _, err = json.Marshal(record); err != nil {
		return record, err
	}

	return record, nil
}
