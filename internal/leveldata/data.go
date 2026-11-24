// Package leveldata defines GPU-ready baked-level manifests and UV layouts.
package leveldata

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"math"
	"strings"
	"unicode/utf8"
)

const (
	Schema           = "karty.baked-level@1"
	Schema2          = "karty.baked-level@2"
	Schema3          = "karty.baked-level@3"
	Schema4          = "karty.baked-level@4"
	Schema5          = "karty.baked-level@5"
	Schema6          = "karty.baked-level@6"
	Schema7          = "karty.baked-level@7"
	Schema8          = "ashura.baked-level@8"
	MaxManifestBytes = 8 << 20
	MaxChunks        = 4096
	MaxPages         = 8192
	MaxPayloadBytes  = 128 << 20
)

var ErrFormat = errors.New("invalid prepared baked level")

// Ref identifies immutable prepared bytes. Paths are relative to the public
// asset root; a chunk bundle can supply its indexed page payloads internally.
type Ref struct {
	Path   string `json:"path"`
	Size   int    `json:"size"`
	SHA256 string `json:"sha256"`
}

// BundleEntry locates one chunk-owned payload inside a verified bundle. Size
// and expected content identity come from the chunk's ordinary page references.
type BundleEntry struct {
	SHA256 string `json:"sha256"`
	Offset int64  `json:"offset"`
}

// ChunkBundle indexes offline-packed geometry, UVs and lightmaps. Material
// textures remain independent shared references. Runtime readers use these
// precomputed offsets without parsing or extracting the archive.
type ChunkBundle struct {
	File    Ref           `json:"file"`
	Entries []BundleEntry `json:"entries"`
}

// Material references shared GPU-ready Source textures.
type Material struct {
	Name           string     `json:"name"`
	Color          [4]float32 `json:"color"`
	DoubleSided    bool       `json:"doubleSided,omitempty"`
	Albedo         *Ref       `json:"albedo,omitempty"`
	AlbedoEncoding string     `json:"albedoEncoding,omitempty"`
	SSBump         *Ref       `json:"ssbump,omitempty"`
}

// Page binds exact geometry/UV1 to combined direct, environment and GI maps.
type Page struct {
	// Preview identifies a build that has not passed final lighting qualification.
	Preview bool `json:"preview,omitempty"`
	// Representation declares calibrated diffuse units in schema v6.
	Representation string `json:"representation,omitempty"`
	// BakeInputs optionally exposes world-space developer inputs. Production
	// loading ignores it; its model digest does not bind localized Geometry.
	BakeInputs *Ref       `json:"bakeInputs,omitempty"`
	Geometry   Ref        `json:"geometry"`
	UV         Ref        `json:"uv"`
	Materials  []Material `json:"materials"`
	Maps       [3]Ref     `json:"maps"`
	Scale      float32    `json:"scale"`
	Encoding   string     `json:"encoding"`
	Storage    string     `json:"storage,omitempty"`
	Width      uint32     `json:"width"`
	Height     uint32     `json:"height"`
}

// Chunk owns one logical space's grid cell. Geometry vertices are local to
// Origin; Origin, Min and Max are world-space coordinates for selection/culling.
type Chunk struct {
	ID      string       `json:"id"`
	Space   string       `json:"space"`
	Cell    [3]int32     `json:"cell"`
	Origin  [3]float32   `json:"origin"`
	Min     [3]float32   `json:"min"`
	Max     [3]float32   `json:"max"`
	Sources []string     `json:"sources"`
	Pages   []Page       `json:"pages"`
	Bundle  *ChunkBundle `json:"bundle,omitempty"`
}

// Object preserves logical identity independently of static presentation residency.
type Object struct {
	ID        string      `json:"id"`
	SaveID    string      `json:"saveId,omitempty"`
	Model     string      `json:"model,omitempty"`
	Dynamic   bool        `json:"isDynamic,omitempty"`
	Transform [16]float32 `json:"transform"`
	Asset     *Ref        `json:"asset,omitempty"`
}

// Document is the bounded, versioned runtime index. Source YAML and bake transport stay offline.
type Document struct {
	Schema        string     `json:"schema"`
	Source        string     `json:"source"`
	ChunkSize     [3]float32 `json:"chunkSize"`
	TexelsPerUnit float32    `json:"texelsPerUnit"`
	Chunks        []Chunk    `json:"chunks"`
	Objects       []Object   `json:"objects"`
}

// Decode admits the complete manifest before any assets are requested.
func Decode(data []byte) (Document, error) {
	if len(data) == 0 || len(data) > MaxManifestBytes {
		return Document{}, ErrFormat
	}

	if err := uniqueJSONKeys(data); err != nil {
		return Document{}, err
	}

	var doc Document

	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()

	if err := decoder.Decode(&doc); err != nil {
		return Document{}, errors.Join(ErrFormat, err)
	}

	if decoder.Decode(new(any)) != io.EOF {
		return Document{}, ErrFormat
	}

	if err := Validate(doc); err != nil {
		return Document{}, err
	}

	return doc, nil
}

// Encode serializes an admitted immutable build result.
func Encode(doc Document) ([]byte, error) {
	if err := Validate(doc); err != nil {
		return nil, err
	}

	data, err := json.Marshal(doc)
	if len(data) > MaxManifestBytes {
		return nil, ErrFormat
	}

	return data, err
}

// Reference computes the immutable external identity of one bounded payload.
func Reference(path string, data []byte) (Ref, error) {
	sum := sha256.Sum256(data)
	ref := Ref{Path: path, Size: len(data), SHA256: hex.EncodeToString(sum[:])}

	return ref, ValidateRef(ref)
}

func ValidateRef(ref Ref) error {
	if !fs.ValidPath(ref.Path) || ref.Path == "." || !validText(ref.Path) || strings.ContainsAny(ref.Path, "\\%?#:\x00") || ref.Size < 1 ||
		ref.Size > MaxPayloadBytes ||
		len(ref.SHA256) != 64 {
		return ErrFormat
	}

	sum, err := hex.DecodeString(ref.SHA256)
	if err != nil || hex.EncodeToString(sum) != ref.SHA256 {
		return ErrFormat
	}

	return nil
}

func finite(v float32) bool { return !math.IsNaN(float64(v)) && !math.IsInf(float64(v), 0) }

func validText(value string) bool {
	if value == "" || len(value) > 4096 || !utf8.ValidString(value) {
		return false
	}

	for _, character := range value {
		if character < ' ' || character == 127 {
			return false
		}
	}

	return true
}
