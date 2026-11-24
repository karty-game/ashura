// Package particledata defines the prepared particle bundle and its GPU layout.
package particledata

import (
	"encoding/binary"
	"errors"
	"io"
	"math"

	"github.com/karty-game/ashura/internal/texturedata"
	"github.com/karty-game/ashura/internal/uipaint"
)

const (
	MaxLayers         = 8
	MaxParticles      = 10000
	MaxBursts         = 32
	MaxPageSide       = 1024
	Samples           = 256
	ConfigVectors     = 7
	SpatialOffset     = (ConfigVectors + 2*Samples) * 16
	RenderOffset      = SpatialOffset + 16
	PlacementOffset   = RenderOffset + 16
	ConfigBytes       = PlacementOffset + 16
	VelocityLocation  = 2
	VelocityOffset    = 16
	ParticleAgeOffset = 32
	FrameBinding      = 2
	QuadVertices      = 6
	FrameBytes        = 32
	StateHeader       = 16
	ParticleBytes     = 48
	WorkgroupSize     = 64
	MaxBundleBytes    = 5 << 20
	MaxWarmParticles  = 4096
	MaxStretch        = 32
)

var (
	ErrBundle = errors.New("particle: invalid prepared bundle")
	ErrGPU    = errors.New("particle GPU resource creation failed")
)

type Blend uint32

const (
	Normal Blend = iota
	Add
)

const (
	SpatialPlanar = iota
	SpatialSphere
	SpatialCone
	SpatialGround
)

type Burst struct {
	Time  float64
	Count uint32
}

type Layer struct {
	Capacity             uint32
	Blend                Blend
	Rate, Delay, MaxLife float64
	Bursts               []Burst
	Config               []byte
	Warm                 []byte
	WarmLife             float64
}

func (layer *Layer) WarmSlots() uint32 {
	if len(layer.Warm) == 0 {
		return 0
	}

	return uint32((len(layer.Warm) - StateHeader) / ParticleBytes) //nolint:gosec // Warm records are bounded at admission.
}

type Effect struct {
	Duration float64
	Loop     bool
	Layers   []Layer
	Page     *uipaint.Texture
}

const (
	magicV1 = 0x01525041 // APR followed by the binary version.
	magicV2 = 0x02525041
	magicV3 = 0x03525041
)

type header struct {
	Magic                       uint32
	Width, Height, Layers, Loop uint32
	Duration                    float64
}

type layerHeader struct {
	Capacity             uint32
	Blend                Blend
	Bursts               uint32
	Rate, Delay, MaxLife float64
}

//nolint:gosec // The bounded offline compiler supplies all bundle dimensions and counts.
func Write(writer io.Writer, effect *Effect) error {
	head := header{
		Magic: magicV3, Width: uint32(effect.Page.Width), Height: uint32(effect.Page.Height),
		Layers: uint32(len(effect.Layers)), Duration: effect.Duration,
	}
	if effect.Loop {
		head.Loop = 1
	}

	if err := binary.Write(writer, binary.LittleEndian, head); err != nil {
		return err
	}

	for _, layer := range effect.Layers {
		info := layerHeader{
			Capacity: layer.Capacity, Blend: layer.Blend, Bursts: uint32(len(layer.Bursts)),
			Rate: layer.Rate, Delay: layer.Delay, MaxLife: layer.MaxLife,
		}
		if err := binary.Write(writer, binary.LittleEndian, info); err != nil {
			return err
		}

		if err := binary.Write(writer, binary.LittleEndian, layer.Bursts); err != nil {
			return err
		}

		if _, err := writer.Write(layer.Config); err != nil {
			return err
		}

		if err := binary.Write(writer, binary.LittleEndian, layer.WarmSlots()); err != nil {
			return err
		}

		if len(layer.Warm) != 0 {
			if _, err := writer.Write(layer.Warm); err != nil {
				return err
			}
		}
	}

	_, err := writer.Write(effect.Page.Pixels)

	return err
}

func Read(reader io.Reader) (*Effect, error) {
	bounded := &io.LimitedReader{R: reader, N: MaxBundleBytes + 1}

	var head header
	if err := binary.Read(bounded, binary.LittleEndian, &head); err != nil {
		return nil, err
	}

	if (head.Magic != magicV1 && head.Magic != magicV2 && head.Magic != magicV3) ||
		head.Width == 0 || head.Height == 0 || head.Width > MaxPageSide ||
		head.Height > MaxPageSide || head.Layers == 0 || head.Layers > MaxLayers ||
		head.Loop > 1 || !between(head.Duration, .05, 3600) {
		return nil, ErrBundle
	}

	effect := &Effect{Duration: head.Duration, Loop: head.Loop != 0, Layers: make([]Layer, head.Layers)}
	for index := range effect.Layers {
		layer, err := readLayer(bounded, head.Duration, head.Magic)
		if err != nil {
			return nil, err
		}

		effect.Layers[index] = layer
	}

	page := &uipaint.Texture{Width: int(head.Width), Height: int(head.Height), Format: texturedata.RGBA8}

	page.Pixels = make([]byte, page.Width*page.Height*4)
	if _, err := io.ReadFull(bounded, page.Pixels); err != nil {
		return nil, err
	}

	var tail [1]byte
	if _, err := io.ReadFull(bounded, tail[:]); err != io.EOF || bounded.N == 0 {
		return nil, ErrBundle
	}

	effect.Page = page

	return effect, nil
}

func readLayer(reader io.Reader, duration float64, version uint32) (Layer, error) {
	var info layerHeader
	if err := binary.Read(reader, binary.LittleEndian, &info); err != nil {
		return Layer{}, err
	}

	if info.Capacity == 0 || info.Capacity > MaxParticles || info.Blend > Add || info.Bursts > MaxBursts ||
		!between(info.Rate, 0, MaxParticles) || !between(info.Delay, 0, duration) ||
		!between(info.MaxLife, .001, 3600) {
		return Layer{}, ErrBundle
	}

	layer := Layer{Capacity: info.Capacity, Blend: info.Blend, Rate: info.Rate, Delay: info.Delay, MaxLife: info.MaxLife}
	layer.Bursts = make([]Burst, info.Bursts)

	if err := binary.Read(reader, binary.LittleEndian, layer.Bursts); err != nil {
		return Layer{}, err
	}

	for _, burst := range layer.Bursts {
		if !between(burst.Time, info.Delay, duration) || burst.Count == 0 || burst.Count > MaxParticles {
			return Layer{}, ErrBundle
		}
	}

	layer.Config = make([]byte, ConfigBytes)

	configBytes := ConfigBytes

	switch version {
	case magicV1:
		configBytes = SpatialOffset
	case magicV2:
		configBytes = RenderOffset
	}

	if _, err := io.ReadFull(reader, layer.Config[:configBytes]); err != nil {
		return Layer{}, err
	}

	if !validConfig(layer.Config, info.MaxLife) {
		return Layer{}, ErrBundle
	}

	if version == magicV3 {
		if err := layer.readWarm(reader); err != nil {
			return Layer{}, err
		}
	}

	return layer, nil
}

func validConfig(config []byte, maxLife float64) bool {
	for offset := 0; offset < len(config); offset += 4 {
		if offset == 6*16 {
			continue // The seed is stored as an unsigned integer.
		}

		value := math.Float32frombits(binary.LittleEndian.Uint32(config[offset:]))
		if !between(float64(value), -1e6, 1e6) {
			return false
		}
	}

	lifeMin := math.Float32frombits(binary.LittleEndian.Uint32(config))
	lifeMax := math.Float32frombits(binary.LittleEndian.Uint32(config[4:]))
	octaves := math.Float32frombits(binary.LittleEndian.Uint32(config[19*4:]))

	return lifeMin > 0 && lifeMax >= lifeMin && float64(lifeMax) <= maxLife && octaves >= 0 && octaves <= 3 &&
		validSpatial(config[SpatialOffset:RenderOffset]) && validRender(config[RenderOffset:PlacementOffset]) &&
		floatAt(config, PlacementOffset+8) >= 0 && floatAt(config, PlacementOffset+12) >= 0
}

func validRender(config []byte) bool {
	mode, speed, low, high := floatAt(config, 0), floatAt(config, 4), floatAt(config, 8), floatAt(config, 12)
	if mode == 0 {
		return speed == 0 && low == 0 && high == 0
	}

	return (mode == 1 || mode == 2) && speed >= 0 && low > 0 && low <= high && high <= MaxStretch
}

func floatAt(data []byte, offset int) float32 {
	return math.Float32frombits(binary.LittleEndian.Uint32(data[offset:]))
}

func (layer *Layer) readWarm(reader io.Reader) error {
	var count uint32
	if err := binary.Read(reader, binary.LittleEndian, &count); err != nil {
		return err
	}

	if count > layer.Capacity || count > MaxWarmParticles {
		return ErrBundle
	}

	if count == 0 {
		return nil
	}

	layer.Warm = make([]byte, StateHeader+int(count)*ParticleBytes)
	if _, err := io.ReadFull(reader, layer.Warm); err != nil {
		return err
	}

	for offset := StateHeader; offset < len(layer.Warm); offset += ParticleBytes {
		record := layer.Warm[offset : offset+ParticleBytes]
		for component := 0; component < ParticleBytes; component += 4 {
			value := floatAt(record, component)
			if !between(float64(value), -1e12, 1e12) {
				return ErrBundle
			}
		}

		age, life := floatAt(record, ParticleAgeOffset), floatAt(record, ParticleAgeOffset+4)
		if age < 0 || age >= life || float64(life) > layer.MaxLife || floatAt(record, ParticleAgeOffset+8) < 0 {
			return ErrBundle
		}

		layer.WarmLife = max(layer.WarmLife, float64(life)-float64(age))
	}

	return nil
}
func validSpatial(config []byte) bool {
	var values [4]float32
	for i := range values {
		values[i] = math.Float32frombits(binary.LittleEndian.Uint32(config[i*4:]))
	}

	mode, radius, cosine, height := values[0], values[1], values[2], values[3]
	switch mode {
	case SpatialPlanar:
		return radius == 0 && cosine == 0 && height == 0
	case SpatialSphere:
		return radius >= 0 && cosine == 0
	case SpatialCone:
		return radius >= 0 && cosine >= 0 && cosine <= 1
	case SpatialGround:
		return radius == 0 && cosine == 0
	default:
		return false
	}
}

func between(value, low, high float64) bool { return value >= low && value <= high }
