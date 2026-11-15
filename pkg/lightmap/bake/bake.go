//go:build !js

// Package lightmapbake prepares static diffuse lighting offline. It has one
// transport model for RGB and RNM outputs and is not a game-runtime dependency.
package lightmapbake

import (
	"context"

	"github.com/karty-game/karty-webgpu/internal/lightmapper"
)

type Vec3 = lightmapper.Vec3
type Vec2 = lightmapper.Vec2
type Triangle = lightmapper.Triangle
type Texture = lightmapper.Texture
type Material = lightmapper.Material
type Emitter = lightmapper.Emitter
type EmitterKind = lightmapper.EmitterKind
type EmissionModel = lightmapper.EmissionModel
type Environment = lightmapper.Environment
type Scene = lightmapper.Scene
type PreparedScene = lightmapper.PreparedScene
type Receiver = lightmapper.Receiver
type FootprintPiece = lightmapper.FootprintPiece
type Options = lightmapper.Options
type Outputs = lightmapper.Outputs
type Estimate = lightmapper.Estimate
type Result = lightmapper.Result

const (
	Point            = lightmapper.Point
	Spot             = lightmapper.Spot
	Rectangle        = lightmapper.Rectangle
	Disk             = lightmapper.Disk
	Sphere           = lightmapper.Sphere
	EmissiveTriangle = lightmapper.EmissiveTriangle
	Directional      = lightmapper.Directional
	Sun              = lightmapper.Sun
	Physical         = lightmapper.Physical
	LegacyRangeV1    = lightmapper.LegacyRangeV1
	OutputRGB        = lightmapper.OutputRGB
	OutputRNM        = lightmapper.OutputRNM
)

var (
	ErrInvalid     = lightmapper.ErrInvalid
	ErrBounds      = lightmapper.ErrBounds
	ErrUnsupported = lightmapper.ErrUnsupported
)

// PrepareScene validates and privately owns reusable build-scene data.
func PrepareScene(scene Scene) (*PreparedScene, error) { return lightmapper.PrepareScene(scene) }

// Reference computes the exhaustive or hierarchy-sampled correctness reference.
// Production callers use the hardware compute Worker instead.
func Reference(ctx context.Context, scene *PreparedScene, receivers []Receiver, options Options) (Result, error) {
	return lightmapper.BakeReference(ctx, scene, receivers, options)
}
