// Package lightmapper implements build-only diffuse light transport. No solver
// or prepared scene is a game-runtime dependency.
package lightmapper

import "errors"

const (
	SceneSchema  = "karty.lightmapper-scene@3"
	Algorithm    = "diffuse-lightmapper@1"
	ResultSchema = "karty.lightmapper-linear@1"
)

var (
	ErrInvalid     = errors.New("lightmapper: invalid prepared input")
	ErrBounds      = errors.New("lightmapper: representation or memory bound exceeded")
	ErrUnsupported = errors.New("lightmapper: unsupported authored feature")
)

type Vec3 [3]float32
type Vec2 [2]float32

type Triangle struct {
	ID                 uint64
	Positions, Normals [3]Vec3
	UV                 [3]Vec2
	Material           uint32
}

// Texture stores linear RGB. UV evaluation repeats and bilinearly filters at
// the base level (linear-repeat-bilinear-v1); output lightmap packing is unrelated.
type Texture struct {
	Width, Height uint32
	Pixels        []Vec3
}

type Material struct {
	ID                    string // Optional stable build identity; unique when nonempty.
	Reflectance, Emission Vec3
	Texture               uint32 // Index plus one; zero means no reflectance texture.
	DoubleSided           bool
}

type EmitterKind uint32

const (
	Point EmitterKind = iota + 1
	Spot
	Rectangle
	Disk
	Sphere
	EmissiveTriangle
	Directional
	Sun
	EnvironmentSource
)

type EmissionModel uint32

const (
	Physical EmissionModel = iota
	LegacyRangeV1
)

// Emitter.Intensity is radiant intensity for point/spot, radiance for surfaces
// and sun, and irradiance for a delta directional source. Direction points away
// from the source; Tangent fixes rectangle/disk orientation. Spot uses a unit-
// peak smoothstep angular profile between OuterCos and InnerCos. Range=0 means
// infinite physical support; legacy sources require positive world-space Range.
// Sun Radius is angular radius in radians. Other radii/sizes use world units.
type Emitter struct {
	ID                                uint64
	Kind                              EmitterKind
	Model                             EmissionModel
	Position, Direction, Tangent      Vec3
	Intensity                         Vec3
	Size                              Vec2 // Full rectangle width and height.
	Radius, Range, InnerCos, OuterCos float32
	TwoSided                          bool
	Triangle                          uint32
}

// Environment uses radiance. LegacyIrradiance explicitly divides authored sky
// and ground colors by pi. Texture, if nonzero, is a latitude-longitude linear
// radiance texture and replaces the sky/ground gradient.
type Environment struct {
	Sky, Ground      Vec3
	Texture          uint32
	LegacyIrradiance bool
}

type Scene struct {
	ID, Space     string
	MetresPerUnit float32
	Triangles     []Triangle
	Materials     []Material
	Textures      []Texture
	Emitters      []Emitter
	Environment   Environment
}

type BVHNode struct {
	Min, Max                  Vec3
	Left, Right, First, Count uint32
}
type LightNode struct {
	Min, Max                  Vec3
	Left, Right, First, Count uint32
	Power                     float32
	Direction                 Vec3
	ConeCos                   float32 // -1 denotes unrestricted emission directions.
	GlobalPower               float32 // Directional/environment incoming-energy proposal, independent of spatial bounds.
}

type Outputs uint32

const (
	OutputRGB Outputs = 1 << iota
	OutputRNM
)

// Options fixes production sample counts before estimation. Adaptive pilot
// allocation is a separate operation; a CPU reference never silently adapts.
type Options struct {
	Outputs                Outputs
	Samples, Bounces, Seed uint32
	RouletteDepth          uint32 // Zero disables compensated roulette.
	ExactLights            bool   // Exhaustive correctness reference; production samples the hierarchy.
}

// Estimate keeps physical D means and second moments in linear floating point.
// Statistics[0] is flat RGB; Statistics[1..3] are the fixed RNM fit RHS.
// Diagnostic channels use the same flat D: direct emitters, environment, bounce.
type Estimate struct {
	ReceiverID                        uint64
	Count                             uint32
	Statistics, SecondMoments         [4]Vec3
	Variance                          [4]Vec3 // Unbiased stable Welford sample variance.
	VarianceValid, SecondMomentsValid bool
	RNM                               [3]Vec3 // Resolved calibrated coefficients; refit after reconstruction.
	Direct, Environment, Reflected    Vec3
}

type Result struct {
	Schema, Algorithm, SceneHash, ReceiverHash string
	Options                                    Options
	Estimates                                  []Estimate
	Backend                                    string
}

// ResponseEstimate retains independently traced full-normal responses and their
// second moments for representation verification, not fitted RNM predictions.
type ResponseEstimate struct {
	ReceiverID                        uint64
	Count                             uint32
	Means, SecondMoments              []Vec3
	Variance                          []Vec3
	VarianceValid, SecondMomentsValid bool
}
