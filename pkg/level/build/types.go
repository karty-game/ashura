//go:build !js

// Package levelbuild compiles authored scenes into externally delivered static chunks and RNM pages.
package levelbuild

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/karty-game/karty-webgpu/internal/leveldata"
	"github.com/karty-game/karty-webgpu/pkg/assetbuild"
	"github.com/karty-game/karty-webgpu/pkg/lightmap"
	scenebuild "github.com/karty-game/karty-webgpu/pkg/scene/build"
)

const (
	rgbeEncoding      = "rgbe8"
	importSchema      = "karty.level-import@1"
	chunksSchema      = "karty.level-chunks@1"
	uvSchema          = "karty.level-uv@1"
	bakedSchema       = "karty.level-baked@1"
	slot              = "manifest"
	filesSlot         = "files"
	filesSchema       = "karty.files@1"
	renderPagesSchema = "karty.level-render-pages@1"
)

// ErrInvalid identifies unsupported or inconsistent authored level inputs.
var ErrInvalid = errors.New("invalid level bake pipeline")

// ImportOptions adapts existing scene YAML and prefabs. Modules defaults to logic+render3d.
// SSBump maps prepared model identity -> original primitive name -> prepared SSBump resource identity.
// Spaces maps node paths to logical spaces; descendants inherit the nearest explicit assignment.
type ImportOptions struct {
	// SeparateMasked keeps static alpha-masked parts outside the opaque bake.
	// Their independent prepared models contain world-space geometry and use an
	// identity placement; masked shadows remain outside this opaque-bake contract.
	SeparateMasked bool

	Input   []string
	Modules []scenebuild.Module
	SSBump  map[string]map[string]string
	Spaces  map[string]string
}

// ChunkOptions configures deterministic spatial partitioning within logical spaces.
type ChunkOptions struct {
	Size, Origin            [3]float32
	MaxChunks, MaxTriangles uint32
}

// UVOptions controls physical density independently of the portable atlas cap.
type UVOptions struct {
	// Production selects sparse footprint inputs without legacy visibility duplication.
	Production                             bool
	TexelsPerUnit                          float32
	MaxPageSize, Padding, TransportSamples uint32
	// PreserveGeometricNormals prepares exact build-only ray normals instead
	// of coarse one-bounce patches for the production path tracer.
	PreserveGeometricNormals bool
}

// PublishOptions selects a small generated Go descriptor package and optional developer bake inputs.
type PublishOptions struct {
	sharedFiles   map[string]fileRecord
	sharedCodecs  *codecActionCache
	sharedAtlases *atlasActionCache
	sharedPages   *packageActionCache
	// Variant namespaces publication actions and generated descriptor names.
	// Empty preserves the original descriptor; otherwise use a Go identifier.
	Variant string
	// Representation selects a production RGB or RNM result without retracing.
	Representation string
	// NativeHDR stores RNM and SSBump as prepared GPU blocks. Its Family
	// selects BC or ETC2/ASTC delivery; no runtime codec is required.
	NativeHDR *NativeHDROptions
	// NativeTextures selects normalized LDR lightmaps and native material maps.
	// It is mutually exclusive with legacy NativeHDR and requires no runtime codec.
	NativeTextures *NativeTextureOptions
	// AlbedoETC1S opts into baked-level@3 web delivery and a load-time block
	// transcode. RNM and SSBump keep their original numeric encoding.
	AlbedoETC1S *ETC1SOptions
	Package     string
	Development bool
	// MaxAtlasSize bounds offline consolidation of baked charts. Zero selects
	// 1024; 2048 opts into baked-level@2. Development retains individual inputs.
	MaxAtlasSize uint32
}

//nolint:tagliatelle // Preserve the existing v1 build-cache record field names.
type placement struct {
	ID        string                         `json:"ID"`
	Space     string                         `json:"Space"`
	Model     string                         `json:"Model"`
	Transform [16]float32                    `json:"Transform"`
	Dynamic   bool                           `json:"Dynamic"`
	Asset     assetbuild.Artifact            `json:"Asset"`
	SSBump    map[string]assetbuild.Artifact `json:"SSBump"`
}

//nolint:tagliatelle // Preserve the existing v1 build-cache record field names.
type staticLight struct {
	Space string         `json:"Space"`
	Point lightmap.Point `json:"Point"`
}

//nolint:tagliatelle // Preserve the existing v1 build-cache record field names.
type imported struct {
	Source      string             `json:"Source"`
	Objects     []leveldata.Object `json:"Objects"`
	Placements  []placement        `json:"Placements"`
	Points      []staticLight      `json:"Points"`
	SkyColor    [3]float32         `json:"SkyColor"`
	GroundColor [3]float32         `json:"GroundColor"`
}

//nolint:tagliatelle // Preserve the existing v1 build-cache record field names.
type chunkRecord struct {
	Description leveldata.Chunk                `json:"Description"`
	Geometry    assetbuild.Artifact            `json:"Geometry"`
	Occluders   assetbuild.Artifact            `json:"Occluders"`
	SSBump      map[string]assetbuild.Artifact `json:"SSBump"`
	Pages       []pageRecord                   `json:"Pages"`
}

//nolint:tagliatelle // Preserve the existing v1 build-cache record field names.
type pageRecord struct {
	Quality          assetbuild.Artifact            `json:"Quality,omitzero"`
	Preview          bool                           `json:"Preview,omitempty"`
	UVLayout         assetbuild.Artifact            `json:"UVLayout,omitzero"`
	Receivers        assetbuild.Artifact            `json:"Receivers,omitzero"`
	RGBMap           assetbuild.Artifact            `json:"RGBMap,omitzero"`
	Representation   string                         `json:"Representation,omitempty"`
	Geometry         assetbuild.Artifact            `json:"Geometry"`
	BakeInputs       assetbuild.Artifact            `json:"BakeInputs"`
	GeometricNormals assetbuild.Artifact            `json:"GeometricNormals"`
	Maps             [3]assetbuild.Artifact         `json:"Maps"`
	Scale            float32                        `json:"Scale"`
	Encoding         string                         `json:"Encoding"`
	Width            uint32                         `json:"Width"`
	Height           uint32                         `json:"Height"`
	SSBump           map[string]assetbuild.Artifact `json:"SSBump"`
}

//nolint:tagliatelle // Preserve the existing v1 build-cache record field names.
type partitioned struct {
	Source      string             `json:"Source"`
	Size        [3]float32         `json:"Size"`
	Density     float32            `json:"Density"`
	Objects     []leveldata.Object `json:"Objects"`
	Dynamic     []placement        `json:"Dynamic"`
	Points      []staticLight      `json:"Points"`
	Chunks      []chunkRecord      `json:"Chunks"`
	SkyColor    [3]float32         `json:"SkyColor"`
	GroundColor [3]float32         `json:"GroundColor"`
}

func encode(value any) ([]byte, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}

	if len(data) > leveldata.MaxManifestBytes {
		return nil, ErrInvalid
	}

	return data, nil
}
func readRecord(ctx *assetbuild.Context, ref assetbuild.Ref, value any) error {
	data, err := ctx.Input(ref)
	if err != nil {
		return err
	}

	return json.Unmarshal(data, value)
}
func result(value any) (assetbuild.Result, error) {
	data, err := encode(value)

	return assetbuild.Result{slot: data}, err
}
func replaceResource(plan *assetbuild.Plan, identity string, ref assetbuild.Ref) {
	for i := range plan.Resources {
		if plan.Resources[i].ID == identity {
			plan.Resources[i].Ref = ref

			return
		}
	}

	plan.Resources = append(plan.Resources, assetbuild.Resource{ID: identity, Ref: ref})
}

type stage struct {
	name, schema string
	run          func(*assetbuild.Plan, assetbuild.Resource) error
}

func (plugin stage) Name() string { return "level-" + plugin.name }
func (plugin stage) Plan(plan *assetbuild.Plan) error {
	return plan.Expand(
		assetbuild.Expansion{ID: "level/" + plugin.name + "/discover", Run: func(_ *assetbuild.Context, child *assetbuild.Plan) error {
			found := false

			for _, resource := range append([]assetbuild.Resource(nil), child.Resources...) {
				schema, err := child.SlotSchema(resource.Ref)
				if err != nil {
					return err
				}

				if schema != plugin.schema {
					continue
				}

				found = true

				if err := plugin.run(child, resource); err != nil {
					return err
				}
			}

			if !found {
				return fmt.Errorf("%s requires preceding level schema %s: %w", plugin.name, plugin.schema, ErrInvalid)
			}

			return nil
		}},
	)
}
