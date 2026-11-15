// Package impostordata admits offline-prepared multi-view impostor resources.
// Its payloads are already in GPU upload layouts; runtime loading never bakes,
// reconstructs geometry, repacks atlases or regenerates the static view table.
package impostordata

import (
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"unicode/utf8"
)

const (
	// MaxBytes bounds the complete prepared container before admission.
	MaxBytes = 192 << 20
	// MaxViews bounds both the view grid and the fixed GPU uniform array.
	MaxViews = 64
	// UniformSize is the exact static shader descriptor/view-table byte size.
	UniformSize           = uniformHeaderBytes + MaxViews*48
	uniformHeaderBytes    = 96
	metadataWordCount     = 22
	version               = 4
	maxSourceBytes        = 4096
	maxTextureDimension   = 4096
	quadVertexBytes       = 4 * 32
	quadIndexBytes        = 6 * 4
	floatBytes            = 4
	alphaCutoff           = .5
	minimumOctahedralGrid = 4
	maximumOctahedralGrid = 8
)

// Mode selects the offline capture layout and corresponding prepared geometry.
type Mode uint32

const (
	// ModeAngular retains the azimuth/elevation view grid.
	ModeAngular Mode = 0
	// ModeOctahedral uses a full-sphere octahedral capture grid.
	ModeOctahedral Mode = 4
	// ModeLayered stores union intervals and a conservative numeric hierarchy.
	ModeLayered Mode = 5
)

// Storage identifies the prepared texture channels consumed by the shader.
type Storage uint32

const (
	// StorageRGBA8 retains RGB normals/depth alpha, or exact layered numeric data.
	StorageRGBA8 Storage = iota
	// StorageBC1 uses punch-through color and oct-normal R/B with depth G.
	StorageBC1
)

var errFormat = errors.New("invalid prepared impostor container")

// View is an orthonormal object-space orthographic capture basis. Outward points
// toward its capture camera. Right/Up orient the image. PlaneDepth is reserved
// by the v4 wire layout and must be zero for the supported capture modes.
type View struct {
	Right, Up, Outward [3]float32
	PlaneDepth         float32
}

// Document contains the complete bounded v4 resource. Elevations use radians in
// angular mode. Octahedral mode stores its square grid size in Azimuths and
// Elevations; HalfExtents describes the source bounds in object space.
// Color is an sRGB KTX2 atlas. Storage selects the normal/depth channel layout.
// Layered requires exact RGBA8 oct normals, depth16 pairs and a hierarchy.
type Document struct {
	Mode                                                                                        Mode
	Storage                                                                                     Storage
	HalfExtents                                                                                 [3]float32
	Source                                                                                      string
	Center                                                                                      [3]float32
	Radius, Roughness                                                                           float32
	Azimuths, Elevations                                                                        uint32
	ElevationMin, ElevationMax                                                                  float32
	AtlasWidth, AtlasHeight, Columns, CellSize, Resolution, Padding, MipLevels, SourceTriangles uint32
	Views                                                                                       []View
	Vertices, Indices, Color, NormalDepth, Uniform                                              []byte
}

// Encode serializes a build-produced resource. Its optional static uniform
// generation runs offline, never as part of Decode or runtime loading.
func Encode(document Document) ([]byte, error) {
	if len(document.Uniform) == 0 {
		document.Uniform = bakeUniform(document)
	}

	if err := validate(document); err != nil {
		return nil, err
	}

	data := append([]byte("KIMPOST4"), 0, 0, 0, 0)
	binary.LittleEndian.PutUint32(data[8:], version)

	//nolint:gosec // validate bounds the source identity to 4096 bytes.
	data = binary.LittleEndian.AppendUint32(data, uint32(len(document.Source)))
	for _, value := range metadata(document) {
		data = binary.LittleEndian.AppendUint32(data, value)
	}

	data = binary.LittleEndian.AppendUint32(data, uint32(len(document.Views))) //nolint:gosec // validate bounds capture count to MaxViews.
	for _, view := range document.Views {
		for _, axis := range [][3]float32{view.Right, view.Up, view.Outward} {
			for _, value := range axis {
				data = binary.LittleEndian.AppendUint32(data, math.Float32bits(value))
			}
		}

		data = binary.LittleEndian.AppendUint32(data, math.Float32bits(view.PlaneDepth))
	}

	sections := [][]byte{document.Vertices, document.Indices, document.Uniform, document.Color, document.NormalDepth}
	for _, section := range sections {
		//nolint:gosec // validate bounds all payload sections below MaxBytes.
		data = binary.LittleEndian.AppendUint32(data, uint32(len(section)))
	}

	data = append(data, document.Source...)
	for _, section := range sections {
		data = append(data, section...)
	}

	if len(data) > MaxBytes {
		return nil, errFormat
	}

	return data, nil
}

// Decode completely validates the bounded binary envelope and immutable static
// metadata before returning views of its prepared byte sections.
func Decode(data []byte) (Document, error) {
	if len(data) > MaxBytes || len(data) < 16 || string(data[:8]) != "KIMPOST4" || binary.LittleEndian.Uint32(data[8:]) != version {
		return Document{}, errFormat
	}

	reader := binaryReader{data: data[12:]}
	sourceLength := reader.word()

	var words [metadataWordCount]uint32
	for index := range words {
		words[index] = reader.word()
	}

	document := fromMetadata(words)

	count := reader.word()
	if reader.failed || sourceLength > maxSourceBytes || count > MaxViews {
		return Document{}, errFormat
	}

	document.Views = make([]View, count)
	for index := range document.Views {
		view := &document.Views[index]
		for _, axis := range []*[3]float32{&view.Right, &view.Up, &view.Outward} {
			for channel := range axis {
				axis[channel] = math.Float32frombits(reader.word())
			}
		}

		view.PlaneDepth = math.Float32frombits(reader.word())
	}

	var lengths [5]uint32
	for index := range lengths {
		lengths[index] = reader.word()
	}

	document.Source = string(reader.take(sourceLength))

	sections := []*[]byte{&document.Vertices, &document.Indices, &document.Uniform, &document.Color, &document.NormalDepth}
	for index, target := range sections {
		*target = reader.take(lengths[index])
	}

	if reader.failed || len(reader.data) != 0 {
		return Document{}, errFormat
	}

	if err := validate(document); err != nil {
		return Document{}, err
	}

	return document, nil
}

func validate(document Document) error {
	if !validIdentity(document) || !validCaptureGrid(document) || !validStorage(document) {
		return errFormat
	}

	if err := validateLayout(document); err != nil {
		return err
	}

	for _, value := range document.Center {
		if !finite(value) {
			return errFormat
		}
	}

	if !validHalfExtents(document) {
		return errFormat
	}

	for _, view := range document.Views {
		if !validView(view) || view.PlaneDepth != 0 {
			return fmt.Errorf("capture basis: %w", errFormat)
		}
	}

	if len(document.Vertices) != quadVertexBytes || len(document.Indices) != quadIndexBytes || len(document.Uniform) != UniformSize ||
		len(document.Color) == 0 || len(document.NormalDepth) == 0 ||
		len(document.Color)+len(document.NormalDepth)+len(document.Vertices)+len(document.Indices)+len(document.Uniform) > MaxBytes {
		return errFormat
	}

	return validateUniform(document)
}

func validIdentity(document Document) bool {
	return document.Source != "" && len(document.Source) <= maxSourceBytes && utf8.ValidString(document.Source) &&
		finite(document.Radius) && document.Radius > 0 && finite(document.Roughness) && document.Roughness >= 0 && document.Roughness <= 1
}

func validStorage(document Document) bool {
	return document.Storage == StorageRGBA8 || document.Storage == StorageBC1 && document.Mode != ModeLayered &&
		document.AtlasWidth%4 == 0 && document.AtlasHeight%4 == 0
}

func validCaptureGrid(document Document) bool {
	if document.Mode == ModeLayered {
		return validLayeredGrid(document)
	}

	if document.Mode == ModeOctahedral {
		return document.Azimuths >= minimumOctahedralGrid && document.Azimuths <= maximumOctahedralGrid &&
			document.Elevations == document.Azimuths &&
			document.Columns == document.Azimuths &&
			len(document.Views) == int(document.Azimuths*document.Azimuths) &&
			document.ElevationMin == 0 &&
			document.ElevationMax == 0
	}

	if document.Mode != ModeAngular {
		return false
	}

	if document.Azimuths < 2 || document.Elevations < 2 || document.Azimuths > MaxViews/document.Elevations {
		return false
	}

	if uint64(len(document.Views)) != uint64(document.Azimuths)*uint64(document.Elevations) {
		return false
	}

	if !finite(document.ElevationMin) || !finite(document.ElevationMax) {
		return false
	}

	return document.ElevationMin < document.ElevationMax && document.ElevationMin > -math.Pi/2 && document.ElevationMax < math.Pi/2
}

func validateLayout(document Document) error {
	if document.Mode == ModeLayered {
		if !validLayeredGrid(document) {
			return errFormat
		}

		return nil
	}

	if !validAtlasSize(document) || !validCellSize(document) || !validMipGrid(document) {
		return errFormat
	}

	alignment := uint32(1) << (document.MipLevels - 1)

	//nolint:gosec // validCaptureGrid has already bounded every mode's view count to MaxViews.
	rows := (uint32(len(document.Views)) + document.Columns - 1) / document.Columns
	if document.Padding < alignment || document.CellSize%alignment != 0 || document.AtlasWidth != document.Columns*document.CellSize ||
		document.AtlasHeight != rows*document.CellSize {
		return errFormat
	}

	return nil
}

func validHalfExtents(document Document) bool {
	for _, value := range document.HalfExtents {
		if !finite(value) || value < 0 || value > document.Radius {
			return false
		}

		if document.Mode != ModeAngular && value == 0 {
			return false
		}
	}

	return true
}

func validAtlasSize(document Document) bool {
	return document.AtlasWidth >= 1 && document.AtlasHeight >= 1 &&
		document.AtlasWidth <= maxTextureDimension && document.AtlasHeight <= maxTextureDimension
}

func validCellSize(document Document) bool {
	return document.Resolution >= 2 && document.Resolution <= maxTextureDimension && document.Padding >= 1 &&
		document.Padding <= maxTextureDimension && document.CellSize == document.Resolution+2*document.Padding &&
		document.CellSize <= maxTextureDimension
}

func validMipGrid(document Document) bool {
	return document.Columns >= 1 && document.Columns <= MaxViews && document.MipLevels >= 1 && document.MipLevels <= 13
}

func finite(value float32) bool { return !math.IsNaN(float64(value)) && !math.IsInf(float64(value), 0) }

func validView(view View) bool {
	const tolerance = .002

	for _, axis := range [][3]float32{view.Right, view.Up, view.Outward} {
		for _, value := range axis {
			if !finite(value) {
				return false
			}
		}

		if math.Abs(float64(dot(axis, axis)-1)) > tolerance {
			return false
		}
	}

	return math.Abs(float64(dot(view.Right, view.Up))) < tolerance && math.Abs(float64(dot(view.Right, view.Outward))) < tolerance &&
		math.Abs(float64(dot(view.Up, view.Outward))) < tolerance
}

func dot(left, right [3]float32) float32 {
	return left[0]*right[0] + left[1]*right[1] + left[2]*right[2]
}

type binaryReader struct {
	data   []byte
	failed bool
}

func (reader *binaryReader) word() uint32 {
	value := reader.take(floatBytes)
	if len(value) != floatBytes {
		return 0
	}

	return binary.LittleEndian.Uint32(value)
}
func (reader *binaryReader) take(size uint32) []byte {
	if uint64(size) > uint64(len(reader.data)) {
		reader.failed = true

		return nil
	}

	value := reader.data[:size]
	reader.data = reader.data[size:]

	return value
}
