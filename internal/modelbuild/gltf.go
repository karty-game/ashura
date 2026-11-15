//go:build !js

// Package modelbuild prepares the static opaque/cutout GLB profile entirely offline.
package modelbuild

import "encoding/json"

type gltf struct {
	Asset struct {
		Version    string `json:"version"`
		MinVersion string `json:"minVersion"`
	} `json:"asset"`
	Scene              *int              `json:"scene"`
	Scenes             []scene           `json:"scenes"`
	Nodes              []node            `json:"nodes"`
	Meshes             []gltfMesh        `json:"meshes"`
	Buffers            []buffer          `json:"buffers"`
	BufferViews        []bufferView      `json:"bufferViews"`
	Accessors          []accessor        `json:"accessors"`
	Materials          []gltfMaterial    `json:"materials"`
	Textures           []gltfTexture     `json:"textures"`
	Images             []gltfImage       `json:"images"`
	Samplers           []sampler         `json:"samplers"`
	Animations         []json.RawMessage `json:"animations"`
	Skins              []json.RawMessage `json:"skins"`
	Cameras            []json.RawMessage `json:"cameras"`
	ExtensionsUsed     []string          `json:"extensionsUsed"`
	ExtensionsRequired []string          `json:"extensionsRequired"`
}
type scene struct {
	Nodes []int `json:"nodes"`
}
type node struct {
	Name        string    `json:"name"`
	Mesh        *int      `json:"mesh"`
	Camera      *int      `json:"camera"`
	Skin        *int      `json:"skin"`
	Children    []int     `json:"children"`
	Matrix      []float64 `json:"matrix"`
	Translation []float64 `json:"translation"`
	Rotation    []float64 `json:"rotation"`
	Scale       []float64 `json:"scale"`
	Weights     []float64 `json:"weights"`
}
type gltfMesh struct {
	Name       string      `json:"name"`
	Primitives []primitive `json:"primitives"`
	Weights    []float32   `json:"weights"`
}
type primitive struct {
	Attributes map[string]int    `json:"attributes"`
	Indices    *int              `json:"indices"`
	Material   *int              `json:"material"`
	Mode       *int              `json:"mode"`
	Targets    []json.RawMessage `json:"targets"`
}
type buffer struct {
	ByteLength int    `json:"byteLength"`
	URI        string `json:"uri"`
}
type bufferView struct {
	Buffer     int `json:"buffer"`
	ByteLength int `json:"byteLength"`
	ByteOffset int `json:"byteOffset"`
	ByteStride int `json:"byteStride"`
	Target     int `json:"target"`
}
type accessor struct {
	BufferView    *int            `json:"bufferView"`
	ByteOffset    int             `json:"byteOffset"`
	ComponentType int             `json:"componentType"`
	Count         int             `json:"count"`
	Type          string          `json:"type"`
	Normalized    bool            `json:"normalized"`
	Sparse        json.RawMessage `json:"sparse"`
	Min           []float64       `json:"min"`
	Max           []float64       `json:"max"`
}
type gltfMaterial struct {
	Name                 string            `json:"name"`
	PBRMetallicRoughness pbr               `json:"pbrMetallicRoughness"`
	NormalTexture        *normalTexture    `json:"normalTexture"`
	OcclusionTexture     *occlusionTexture `json:"occlusionTexture"`
	EmissiveTexture      *textureRef       `json:"emissiveTexture"`
	EmissiveFactor       []float32         `json:"emissiveFactor"`
	AlphaMode            string            `json:"alphaMode"`
	AlphaCutoff          *float32          `json:"alphaCutoff"`
	DoubleSided          bool              `json:"doubleSided"`
}
type pbr struct {
	BaseColorFactor          []float32   `json:"baseColorFactor"`
	BaseColorTexture         *textureRef `json:"baseColorTexture"`
	MetallicFactor           *float32    `json:"metallicFactor"`
	RoughnessFactor          *float32    `json:"roughnessFactor"`
	MetallicRoughnessTexture *textureRef `json:"metallicRoughnessTexture"`
}
type textureRef struct {
	Index    int `json:"index"`
	TexCoord int `json:"texCoord"`
}
type normalTexture struct {
	Index    int      `json:"index"`
	TexCoord int      `json:"texCoord"`
	Scale    *float32 `json:"scale"`
}
type occlusionTexture struct {
	Index    int      `json:"index"`
	TexCoord int      `json:"texCoord"`
	Strength *float32 `json:"strength"`
}
type gltfTexture struct {
	Source  *int `json:"source"`
	Sampler *int `json:"sampler"`
}
type gltfImage struct {
	BufferView *int   `json:"bufferView"`
	MimeType   string `json:"mimeType"`
	URI        string `json:"uri"`
}
type sampler struct {
	MagFilter int `json:"magFilter"`
	MinFilter int `json:"minFilter"`
	WrapS     int `json:"wrapS"`
	WrapT     int `json:"wrapT"`
}
