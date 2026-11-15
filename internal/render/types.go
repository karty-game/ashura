// Package render holds the private scene snapshot passed from the ECS to the
// native and browser WebGPU drivers.
package render

import (
	"github.com/karty-game/karty-webgpu/internal/bufferdata"
	"github.com/karty-game/karty-webgpu/internal/computedata"
	"github.com/karty-game/karty-webgpu/internal/impostordata"
	"github.com/karty-game/karty-webgpu/internal/meshdata"
	"github.com/karty-game/karty-webgpu/internal/spritepaint"
	"github.com/karty-game/karty-webgpu/internal/texturedata"
	"github.com/karty-game/karty-webgpu/internal/uipaint"
)

// Vertex is the static mesh vertex format shared by both render backends.
type Vertex struct {
	Position [3]float32
	Normal   [3]float32
	UV       [2]float32
}

// DrawCall describes one indexed mesh draw.
type DrawCall struct {
	Impostor           *impostordata.Document
	ImpostorQuality    [4]float32
	MeshOwner          any
	MeshValid          bool
	ID                 uint32
	Albedo, Normal, AO *texturedata.Texture
	Reflection         *texturedata.Texture
	Lightmap           *texturedata.Texture
	RNM                [3]*texturedata.Texture
	RNMScale           float32
	RNMEncoding        texturedata.RNMEncoding
	LightmapScale      float32
	LightmapEncoding   texturedata.RNMEncoding
	LightmapDiffuse    bool
	LightmapDebug      bool
	AOStrength         float32
	ReflectionStrength float32
	ProbeOrigin        [3]float32
	ProbeMinimum       [3]float32
	ProbeMaximum       [3]float32
	ProbeID            uint32
	UVScale            [2]float32
	NormalStrength     float32
	Prepared           *meshdata.Geometry
	Vertices           []Vertex
	Indices            []uint32
	Transform          [16]float32
	Color              [4]float32
	Roughness          float32
	AlphaMask          bool
	AlphaCutoff        float32
	CullBackfaces      bool
	Billboard          bool
	Source             bool
	HalfLambert        bool
	SSBumpStrength     float32
	AmbientColors      [3][4]float32
	Instance           InstanceTransform
	Instances          []InstanceTransform
	InstanceVersion    uint64
	InstanceOwner      any
	ChangedInstances   []int
	InstanceKey        any
}

// InstanceTransform contains the model matrix for one element of a GPU
// instanced draw. The shader derives the inverse-transpose normal matrix.
type InstanceTransform struct {
	Model [16]float32
}

// Light is one directional or hemisphere light record.
type Light struct {
	Kind             uint32
	Direction        [3]float32
	Color            [3]float32
	GroundColor      [3]float32
	Intensity        float32
	CastShadows      bool
	DebugShadowFaces bool
	ShadowResolution uint32
	ShadowBias       float32
}

const (
	HemisphereLightKind    uint32 = 1
	DirectionalLightKind   uint32 = 2
	PointLightKind         uint32 = 3
	PerspectiveProjection  uint8  = 1
	OrthographicProjection uint8  = 2
)

// Frame is the complete scene snapshot required for one presentation.
type Frame struct {
	Screen2D *spritepaint.Frame
	UI       *uipaint.Frame
	Uploads  []bufferdata.Upload
	Compute  []computedata.Dispatch

	// PointerLock permits browser pointer capture for the active free-look camera.
	PointerLock    bool
	CameraPosition [3]float32
	ViewProjection [16]float32
	ClearColor     [4]float32
	Draws          []DrawCall
	Lights         []Light
}
