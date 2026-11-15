//go:build !js

// Package impostorbake rasterizes prepared static models into GPU-ready impostor
// views offline. It has no dependency on a graphics device or native renderer.
package impostorbake

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"math"

	"github.com/karty-game/karty-webgpu/internal/impostordata"
	"github.com/karty-game/karty-webgpu/internal/modeldata"
	"github.com/karty-game/karty-webgpu/pkg/assetbuild"
)

const (
	channels                    = 4
	maxDimension                = 4096
	maxViews                    = 64
	defaultResolution           = 128
	defaultOctahedralResolution = 80
	defaultFrameGrid            = 8
	minimumFrameGrid            = 4
	defaultAzimuths             = 8
	defaultElevations           = 3
	defaultPadding              = 4
	defaultMips                 = 3
	defaultElevation            = 30
	degreesToRadians            = math.Pi / 180
	byteMaximum                 = 255
	paddingSides                = 2
	triangleVertices            = 3
	half                        = .5
	geometryEpsilon             = 1e-12
	minimumProjectionSpan       = 1e-6
	minimumAngularSamples       = 2
)

var (
	errBounds      = errors.New("impostor capture settings or model bounds are invalid")
	errUnsupported = errors.New("impostor baker does not support this material or geometry feature")
)

// Options controls independent captures; elevation bounds are in degrees.
// Zero Mode is angular for internal compatibility. Octahedral mode defaults to
// an 8x8 grid of 80px captures. Both modes use padding4 and three mip levels.
type Options struct {
	MaxIntervals int               `json:"maxIntervals,omitempty"`
	FrameGrid    int               `json:"frameGrid,omitempty"`
	Mode         impostordata.Mode `json:"mode"`
	Resolution   int               `json:"resolution"`
	Azimuths     int               `json:"azimuths"`
	Elevations   int               `json:"elevations"`
	Padding      int               `json:"padding"`
	MipLevels    int               `json:"mipLevels"`
	ElevationMin float32           `json:"elevationMin"`
	ElevationMax float32           `json:"elevationMax"`
}

// Defaults resolves zero-value settings and validates all allocation bounds.
func Defaults(options Options) (Options, error) {
	if options.Mode == impostordata.ModeLayered {
		return layeredDefaults(options)
	}

	return viewDefaults(options)
}

func viewDefaults(options Options) (Options, error) {
	if options.Mode == impostordata.ModeOctahedral && options.FrameGrid == 0 {
		options.FrameGrid = defaultFrameGrid
	}

	if options.Resolution == 0 {
		options.Resolution = options.defaultResolution()
	}

	if options.Azimuths == 0 && options.Mode == impostordata.ModeAngular {
		options.Azimuths = defaultAzimuths
	}

	if options.Elevations == 0 && options.Mode == impostordata.ModeAngular {
		options.Elevations = defaultElevations
	}

	if options.Padding == 0 {
		options.Padding = defaultPadding
	}

	if options.MipLevels == 0 {
		options.MipLevels = defaultMips
	}

	if options.ElevationMin == 0 && options.ElevationMax == 0 && options.Mode == impostordata.ModeAngular {
		options.ElevationMin, options.ElevationMax = -defaultElevation, defaultElevation
	}

	if !validCaptureCounts(options) || !validModeSettings(options) {
		return Options{}, errBounds
	}

	alignment := 1 << (options.MipLevels - 1)

	cell := options.Resolution + paddingSides*options.Padding
	if options.Padding < alignment || options.Resolution%alignment != 0 || cell%alignment != 0 ||
		cell*options.columns() > maxDimension || cell*options.rows() > maxDimension {
		return Options{}, fmt.Errorf("capture atlas dimensions or gutter/mip alignment: %w", errBounds)
	}

	return options, nil
}

func (options Options) defaultResolution() int {
	if options.Mode == impostordata.ModeOctahedral {
		return defaultOctahedralResolution
	}

	return defaultResolution
}

func validCaptureCounts(options Options) bool {
	return options.Resolution >= 16 && options.Resolution <= 512 &&
		(options.Mode == impostordata.ModeOctahedral || options.Resolution&(options.Resolution-1) == 0) &&
		options.Padding >= 1 &&
		options.Padding <= 128 &&
		options.MipLevels >= 1 &&
		options.MipLevels <= 8
}

func validModeSettings(options Options) bool {
	if options.MaxIntervals != 0 {
		return false
	}

	if options.Mode == impostordata.ModeOctahedral {
		return options.FrameGrid >= minimumFrameGrid && options.FrameGrid <= defaultFrameGrid && options.Azimuths == 0 &&
			options.Elevations == 0 &&
			options.ElevationMin == 0 &&
			options.ElevationMax == 0
	}

	if options.FrameGrid != 0 {
		return false
	}

	if options.Mode != impostordata.ModeAngular {
		return false
	}

	validAzimuths := options.Azimuths >= minimumAngularSamples && options.Azimuths <= maxViews

	validElevations := options.Elevations >= minimumAngularSamples && options.Elevations <= maxViews
	if !validAzimuths || !validElevations {
		return false
	}

	return options.Azimuths*options.Elevations <= maxViews && validCaptureAngles(options)
}

func (options Options) columns() int {
	if options.Mode == impostordata.ModeOctahedral {
		return options.FrameGrid
	}

	return options.Azimuths
}
func (options Options) rows() int {
	if options.Mode == impostordata.ModeOctahedral {
		return options.FrameGrid
	}

	return options.Elevations
}

func validCaptureAngles(options Options) bool {
	return finite(float64(options.ElevationMin)) && finite(float64(options.ElevationMax)) &&
		options.ElevationMin >= -80 && options.ElevationMax <= 80 && options.ElevationMin < options.ElevationMax
}

// Report records offline layered geometry diagnostics, never runtime state.
type Report struct {
	ToleratedNegativePockets   int
	MaximumNegativePocketDepth float64
	MaximumIntervals           int
}

// Bake captures prepared geometry and discards optional offline diagnostics.
func Bake(ctx context.Context, source modeldata.Document, options Options) (impostordata.Document, error) {
	result, _, err := BakeWithReport(ctx, source, options)

	return result, err
}

// BakeWithReport captures prepared geometry and reports tolerated numeric pockets.
func BakeWithReport(ctx context.Context, source modeldata.Document, options Options) (impostordata.Document, Report, error) {
	var report Report

	result, err := bake(ctx, source, options, &report)

	return result, report, err
}

func bake(ctx context.Context, source modeldata.Document, options Options, report *Report) (impostordata.Document, error) {
	if err := ctx.Err(); err != nil {
		return impostordata.Document{}, err
	}

	options, err := Defaults(options)
	if err != nil {
		return impostordata.Document{}, err
	}

	geometry, err := prepare(source)
	if err != nil {
		return impostordata.Document{}, err
	}

	if options.Mode == impostordata.ModeLayered {
		return bakeLayered(ctx, source, geometry, options, report)
	}

	result := descriptor(source, geometry, options)

	colorLevels, normalLevels, views, err := captureAtlases(ctx, geometry, options)
	if err != nil {
		return impostordata.Document{}, err
	}

	result.Views = views

	result.Color, err = encodeAtlas(source.Source, "ImpostorColor", assetbuild.UsageAlbedo, colorLevels, options)
	if err != nil {
		return impostordata.Document{}, err
	}

	result.NormalDepth, err = encodeAtlas(source.Source, "ImpostorNormalDepth", assetbuild.UsageAO, normalLevels, options)
	if err != nil {
		return impostordata.Document{}, err
	}

	return result, nil
}

//nolint:gosec // Defaults and prepare bound every count/dimension before narrowing into the binary resource metadata.
func descriptor(source modeldata.Document, geometry geometry, options Options) impostordata.Document {
	cell := options.Resolution + options.Padding*paddingSides
	result := impostordata.Document{
		Mode:        options.Mode,
		HalfExtents: floatVector(geometry.halfExtents),
		Source:      source.Source,
		Center:      [3]float32{float32(geometry.center[0]), float32(geometry.center[1]), float32(geometry.center[2])},
		Radius:      float32(geometry.radius),
		Roughness:   geometry.roughness,
		Azimuths:    uint32(options.Azimuths),
		Elevations:  uint32(options.Elevations),
		Columns: uint32(
			options.columns(),
		),
		CellSize:        uint32(cell),
		Resolution:      uint32(options.Resolution),
		Padding:         uint32(options.Padding),
		AtlasWidth:      uint32(cell * options.columns()),
		AtlasHeight:     uint32(cell * options.rows()),
		MipLevels:       uint32(options.MipLevels),
		ElevationMin:    options.ElevationMin * degreesToRadians,
		ElevationMax:    options.ElevationMax * degreesToRadians,
		SourceTriangles: uint32(len(geometry.triangles)),
	}

	if options.Mode == impostordata.ModeOctahedral {
		result.Azimuths, result.Elevations = uint32(options.FrameGrid), uint32(options.FrameGrid)
	}

	result.Vertices, result.Indices = quad()

	return result
}

func captureAtlases(ctx context.Context, geometry geometry, options Options) ([][]byte, [][]byte, []impostordata.View, error) {
	if options.Mode == impostordata.ModeOctahedral {
		return captureOctahedral(ctx, geometry, options)
	}

	colorLevels, normalLevels := atlasLevels(options)

	views := make([]impostordata.View, 0, options.Azimuths*options.Elevations)
	for elevation := range options.Elevations {
		angle := (float64(options.ElevationMin) + float64(elevation)*float64(options.ElevationMax-options.ElevationMin)/float64(options.Elevations-1)) * degreesToRadians
		for azimuth := range options.Azimuths {
			if err := ctx.Err(); err != nil {
				return nil, nil, nil, err
			}

			view := captureView(float64(azimuth)*2*math.Pi/float64(options.Azimuths), angle)
			views = append(views, view)

			colorPixels, normalPixels := raster(ctx, geometry, view, options.Resolution)
			if err := ctx.Err(); err != nil {
				return nil, nil, nil, err
			}

			packView(colorLevels, normalLevels, colorPixels, normalPixels, azimuth, elevation, options)
		}
	}

	return colorLevels, normalLevels, views, nil
}

func captureView(azimuth, elevation float64) impostordata.View {
	outward := vector{math.Sin(azimuth) * math.Cos(elevation), math.Sin(elevation), math.Cos(azimuth) * math.Cos(elevation)}
	right := unit(cross(vector{0, 1, 0}, outward))
	up := unit(cross(outward, right))

	return impostordata.View{Right: floatVector(right), Up: floatVector(up), Outward: floatVector(outward)}
}

func quad() ([]byte, []byte) {
	var vertices, indices []byte

	for _, corner := range [][4]float32{{-1, -1, 0, 1}, {1, -1, 1, 1}, {1, 1, 1, 0}, {-1, 1, 0, 0}} {
		for _, value := range []float32{corner[0], corner[1], 0, 0, 0, 1, corner[2], corner[3]} {
			vertices = binary.LittleEndian.AppendUint32(vertices, math.Float32bits(value))
		}
	}

	for _, index := range []uint32{0, 1, 2, 0, 2, 3} {
		indices = binary.LittleEndian.AppendUint32(indices, index)
	}

	return vertices, indices
}

func encodeAtlas(source, name, usage string, levels [][]byte, options Options) ([]byte, error) {
	cell := options.Resolution + paddingSides*options.Padding

	return assetbuild.EncodeTexture(
		assetbuild.Image{Name: name, Source: source, Usage: usage, Width: cell * options.columns(), Height: cell * options.rows(),
			Levels: levels, MaxMipLevels: options.MipLevels, Atlas: true},
	)
}
