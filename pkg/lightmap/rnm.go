package lightmap

import (
	"github.com/karty-game/karty-webgpu/internal/lightmapdata"
	"github.com/karty-game/karty-webgpu/internal/lightmapkernel"
	"github.com/karty-game/karty-webgpu/pkg/compute"
	"github.com/karty-game/karty-webgpu/pkg/shader"
	"github.com/karty-game/karty-webgpu/pkg/texture"
)

// RNMScene gathers direct, environment and optional one-bounce diffuse lighting
// into the same three directional coefficient maps. See its field documentation
// in lightmapkernel.RNMScene. It is a deterministic approximation to radiosity.
type RNMScene = lightmapkernel.RNMScene

// RNMProgress measures scheduled rows, not GPU completion or elapsed time.
type RNMProgress struct {
	Rows, TotalRows uint32
	Complete        bool
}

// RNMBaker retains the offline-prepared scene and reusable compute resources.
// A changed scene starts a progressive bake; unchanged frames do no CPU or GPU
// work. The previous maps remain intact until the final resolve dispatch.
type RNMBaker struct {
	asset                                        *Asset
	image                                        texture.RNMLightmap
	parameters, points                           *shader.Buffer
	bake, resolve, patches                       *compute.Pipeline
	bakeBindings, resolveBindings, patchBindings *compute.BindGroup
	bytes                                        [lightmapkernel.RNMUniformSize]byte
	scene                                        RNMScene
	rows, rowCount                               uint32
	pendingScale                                 float32
	initialized, pending                         bool
	recordings                                   uint64
}

// NewRNM declares all resources before App.Run. RNM input preparation is offline.
// Its 48-byte-per-pixel scratch buffer currently supports at most 128 MiB;
// 1024-square atlases use 48 MiB of GPU scratch plus CPU staging in this backend.
func NewRNM(owner compute.Owner, asset *Asset) (*RNMBaker, error) {
	if owner == nil || asset == nil || asset.document == nil {
		return nil, errOwner
	}

	parameters, points, _, _, err := lightmapkernel.RNMParameters(asset.document, RNMScene{})
	if err != nil {
		return nil, err
	}

	baker := &RNMBaker{asset: asset, image: texture.RNMLightmap{Scale: 1}}
	if len(asset.document.ReceiverGIIndex) > 0 {
		baker.image.Encoding = texture.RNMRGBE
	}

	baker.parameters, err = shader.NewUniformBytes(owner, parameters[:])
	if err != nil {
		return nil, err
	}

	baker.points, err = compute.NewStorageBytes(owner, points)
	if err != nil {
		return nil, err
	}

	scene, err := compute.NewStorageBytes(owner, asset.document.GPUData)
	if err != nil {
		return nil, err
	}

	width, height := asset.Size()

	scratch, err := owner.NewBuffer(shader.BufferDesc{Label: "RNM coefficient scratch", Usage: shader.StorageBuffer,
		Size: lightmapkernel.RNMScratchSize(asset.document)})
	if err != nil {
		return nil, err
	}

	bindings := []shader.Binding{
		shader.BindBuffer(0, baker.parameters),
		shader.BindBuffer(1, scene),
		shader.BindBuffer(2, baker.points),
		shader.BindBuffer(3, scratch),
	}
	for index := range baker.image.Textures {
		baker.image.Textures[index], err = compute.NewTexture(owner, int(width), int(height))
		if err != nil {
			return nil, err
		}

		bindings = append(bindings, shader.BindTexture(uint32(index+4), baker.image.Textures[index]))
	}

	if err = baker.createPipelines(owner, bindings); err != nil {
		return nil, err
	}

	return baker, nil
}

// SetScene copies an admitted light list once when editor settings change. It
// does not rebuild or repack geometry. Samples default to eight; Bounces is 0/1.
func (baker *RNMBaker) SetScene(scene RNMScene) error {
	if baker == nil {
		return errBaker
	}

	if baker.initialized && sameRNMScene(baker.scene, scene) {
		return nil
	}

	parameters, points, scale, normalized, err := lightmapkernel.RNMParameters(baker.asset.document, scene)
	if err != nil {
		return err
	}

	if err = baker.points.Write(0, points); err != nil {
		return err
	}

	baker.bytes = parameters
	baker.scene = scene
	baker.scene.Points = append([]Point(nil), scene.Points...)
	baker.pendingScale = scale
	baker.rowCount = normalized.RowsPerRecord
	baker.rows = 0
	baker.initialized = true
	baker.pending = true

	return nil
}

func sameRNMScene(first, second RNMScene) bool {
	if first.SkyColor != second.SkyColor || first.GroundColor != second.GroundColor || first.Samples != second.Samples ||
		first.Bounces != second.Bounces ||
		first.Seed != second.Seed ||
		first.RowsPerRecord != second.RowsPerRecord ||
		len(first.Points) != len(second.Points) {
		return false
	}

	for i := range first.Points {
		if first.Points[i] != second.Points[i] {
			return false
		}
	}

	return true
}

// Lightmap returns a persistent linear RNM tuple. Scale and atlas content switch
// together when the final resolve is recorded; prior maps remain usable while baking.
func (baker *RNMBaker) Lightmap() *texture.RNMLightmap { return &baker.image }
func (baker *RNMBaker) Pending() bool                  { return baker.pending }
func (baker *RNMBaker) Recordings() uint64             { return baker.recordings }
func (baker *RNMBaker) Progress() RNMProgress {
	return RNMProgress{Rows: baker.rows, TotalRows: baker.asset.document.Height, Complete: baker.initialized && !baker.pending}
}

// Record schedules one row tile and, on the last tile, chart-local gutter
// resolution. There are no GPU waits or readbacks. Invoke once per frame.
func (baker *RNMBaker) Record(frame Recorder) error {
	if baker == nil {
		return errBaker
	}

	if !baker.pending {
		return nil
	}

	if frame == nil {
		return errRecorder
	}

	width, height := baker.asset.Size()
	count := min(baker.rowCount, height-baker.rows)
	lightmapkernel.RNMRows(&baker.bytes, baker.rows, count)

	if err := baker.parameters.Write(0, baker.bytes[:]); err != nil {
		return err
	}

	if baker.rows == 0 && baker.patches != nil && baker.scene.Bounces > 0 {
		const patchesPerWorkgroup = 64

		//nolint:gosec // Admission bounds the immutable patch array to 128 MiB.
		patchCount := uint32(len(baker.asset.document.GIPatches) / lightmapdata.GIPatchStride)
		if err := frame.Dispatch(
			baker.patches,
			baker.patchBindings,
			(patchCount+patchesPerWorkgroup-1)/patchesPerWorkgroup,
			1,
			1,
		); err != nil {
			return err
		}

		baker.recordings++
	}

	if err := frame.Dispatch(
		baker.bake,
		baker.bakeBindings,
		(width+lightmapkernel.WorkgroupSize-1)/lightmapkernel.WorkgroupSize,
		(count+lightmapkernel.WorkgroupSize-1)/lightmapkernel.WorkgroupSize,
		1,
	); err != nil {
		return err
	}

	baker.recordings++
	if baker.rows+count == height {
		if err := frame.Dispatch(
			baker.resolve,
			baker.resolveBindings,
			(width+lightmapkernel.WorkgroupSize-1)/lightmapkernel.WorkgroupSize,
			(height+lightmapkernel.WorkgroupSize-1)/lightmapkernel.WorkgroupSize,
			1,
		); err != nil {
			return err
		}

		baker.recordings++
		baker.pending = false
		baker.image.Scale = baker.pendingScale
	}

	baker.rows += count

	return nil
}

func (baker *RNMBaker) createPipelines(owner compute.Owner, bindings []shader.Binding) error {
	var err error

	baker.bake, err = compute.NewPipeline(owner, lightmapkernel.RNMDescription(baker.asset.document, "bake_rnm"))
	if err != nil {
		return err
	}

	baker.resolve, err = compute.NewPipeline(owner, lightmapkernel.RNMDescription(baker.asset.document, "resolve_rnm"))
	if err != nil {
		return err
	}

	baker.bakeBindings, err = baker.bake.BindGroup(bindings...)
	if err != nil {
		return err
	}

	baker.resolveBindings, err = baker.resolve.BindGroup(bindings...)
	if err != nil {
		return err
	}

	if len(baker.asset.document.GIPatches) > 0 {
		baker.patches, err = compute.NewPipeline(owner, lightmapkernel.RNMDescription(baker.asset.document, "bake_rnm_patches"))
		if err != nil {
			return err
		}

		baker.patchBindings, err = baker.patches.BindGroup(bindings...)
		if err != nil {
			return err
		}
	}

	return nil
}
