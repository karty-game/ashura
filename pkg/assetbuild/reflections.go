//go:build !js

package assetbuild

import (
	"fmt"
	"image"
	"image/color"
	"math"
	"path"
	"strings"
)

const maxReflectionFaceSize = 512

type reflectionPlugin struct{}

// ReflectionProbes converts equirectangular `*-reflection` images into six-face
// horizontal atlases and independently filtered mip chains. Put it after Images
// and before KTX2 in a project recipe.
func ReflectionProbes() Plugin { return reflectionPlugin{} }

func (reflectionPlugin) Name() string { return "reflection-probes" }

func (reflectionPlugin) Plan(plan *Plan) error {
	for index, resource := range plan.Resources {
		if !reflectionSource(resource.ID) {
			continue
		}

		if err := requireResourceSchema(plan, resource.Ref, ImageSetSchema); err != nil {
			return err
		}

		refs, err := plan.Add(Action{
			ID: imageActionID(plan, "reflection-probes/"+resource.ID), Processor: "reflection-probes", Revision: "equirect-cube-mips@1",
			Options: resource.ID, Inputs: []Ref{resource.Ref}, Outputs: []Slot{{Name: imageDataSlot, Schema: ImageSetSchema}},
			Run: func(ctx *Context) (Result, error) {
				set, inputErr := imageInput(ctx, resource.Ref)
				if inputErr != nil {
					return nil, inputErr
				}

				if len(set.Images) != 1 {
					return nil, fmt.Errorf("reflection input %q must contain one panorama: %w", resource.ID, errImageRecord)
				}

				baked, bakeErr := BakeReflection(set.Images[0])
				if bakeErr != nil {
					return nil, bakeErr
				}

				return imageResult(ImageSet{Images: []Image{baked}})
			},
		})
		if err != nil {
			return err
		}

		plan.Resources[index].Ref = refs[0]
	}

	return nil
}

func reflectionSource(identity string) bool {
	name := strings.ToLower(path.Base(identity))

	return name == UsageReflection || strings.HasSuffix(name, "-"+UsageReflection) || strings.HasSuffix(name, "_"+UsageReflection)
}

// BakeReflection projects an equirectangular sRGB panorama into +X, -X, +Y,
// -Y, +Z, -Z horizontal face order. Mips are filtered independently per face
// in linear light, avoiding cross-face bleed during preparation.
//
//nolint:mnd // Cube projection converts normalized UVs to signed axes and stores six equal faces.
func BakeReflection(value Image) (Image, error) {
	if err := validateImageSet(ImageSet{Images: []Image{value}}); err != nil {
		return Image{}, err
	}

	if value.Usage != UsageReflection || value.Width != value.Height*2 || len(value.Levels) != 1 {
		return Image{}, fmt.Errorf("reflection source must be a base-only 2:1 panorama: %w", errImageRecord)
	}

	faceSize := min(value.Height/2, maxReflectionFaceSize)

	faceSize = highestPowerOfTwo(faceSize)
	if faceSize < 1 || faceSize*6 > maxImageDimension {
		return Image{}, errImageRecord
	}

	source := &image.NRGBA{Pix: value.Levels[0], Stride: value.Width * imageChannels, Rect: image.Rect(0, 0, value.Width, value.Height)}

	faces := make([][]*image.NRGBA, 6)
	for face := range faces {
		base := image.NewNRGBA(image.Rect(0, 0, faceSize, faceSize))
		for y := range faceSize {
			for x := range faceSize {
				u := (float64(x)+0.5)/float64(faceSize)*2 - 1
				v := 1 - (float64(y)+0.5)/float64(faceSize)*2
				direction := faceDirection(face, u, v)
				base.SetNRGBA(x, y, samplePanorama(source, direction))
			}
		}

		faces[face] = mipmaps(base, UsageAlbedo, 0)
	}

	levelCount := len(faces[0])
	value.Width, value.Height = faceSize*6, faceSize

	value.Levels = make([][]byte, levelCount)
	for level := range levelCount {
		levelSize := max(faceSize>>level, 1)

		atlas := image.NewNRGBA(image.Rect(0, 0, levelSize*6, levelSize))
		for face := range faces {
			copyFace(atlas, face*levelSize, faces[face][level])
		}

		value.Levels[level] = atlas.Pix
	}

	return value, nil
}

func highestPowerOfTwo(value int) int {
	result := 1
	for result <= value/2 {
		result *= 2
	}

	return result
}

//nolint:mnd,varnamelen // Cube-face indices and local u/v are the documented six-face projection convention.
func faceDirection(face int, u, v float64) [3]float64 {
	switch face {
	case 0: // +X
		return normalizeDirection([3]float64{1, -v, -u})
	case 1: // -X
		return normalizeDirection([3]float64{-1, -v, u})
	case 2: // +Y
		return normalizeDirection([3]float64{-u, 1, v})
	case 3: // -Y
		return normalizeDirection([3]float64{u, -1, v})
	case 4: // +Z
		return normalizeDirection([3]float64{u, -v, 1})
	default: // -Z
		return normalizeDirection([3]float64{u, v, -1})
	}
}

func normalizeDirection(value [3]float64) [3]float64 {
	length := math.Sqrt(value[0]*value[0] + value[1]*value[1] + value[2]*value[2])

	return [3]float64{value[0] / length, value[1] / length, value[2] / length}
}

//nolint:mnd,varnamelen // Spherical coordinates and bilinear texel fractions retain conventional x/y notation.
func samplePanorama(source *image.NRGBA, direction [3]float64) color.NRGBA {
	u := math.Atan2(direction[2], direction[0])/(2*math.Pi) + 0.5
	v := 0.5 - math.Asin(max(-1, min(1, direction[1])))/math.Pi
	width, height := source.Bounds().Dx(), source.Bounds().Dy()
	x := u*float64(width) - 0.5
	y := max(0, min(float64(height-1), v*float64(height)-0.5))
	x0 := int(math.Floor(x))
	xf := x - math.Floor(x)
	ty0 := int(math.Floor(y))
	yf := y - float64(ty0)

	read := func(px, py int) color.NRGBA {
		px %= width
		if px < 0 {
			px += width
		}

		return source.NRGBAAt(px, max(0, min(height-1, py)))
	}

	a, b := read(x0, ty0), read(x0+1, ty0)
	c, d := read(x0, ty0+1), read(x0+1, ty0+1)
	channel := func(a, b, c, d uint8) uint8 {
		top := float64(a)*(1-xf) + float64(b)*xf
		bottom := float64(c)*(1-xf) + float64(d)*xf

		return uint8(math.Round(top*(1-yf) + bottom*yf))
	}

	return color.NRGBA{
		R: channel(a.R, b.R, c.R, d.R), G: channel(a.G, b.G, c.G, d.G),
		B: channel(a.B, b.B, c.B, d.B), A: channel(a.A, b.A, c.A, d.A),
	}
}

func copyFace(atlas *image.NRGBA, left int, face *image.NRGBA) {
	width, height := face.Bounds().Dx(), face.Bounds().Dy()
	for y := range height {
		from := face.PixOffset(0, y)
		to := atlas.PixOffset(left, y)
		copy(atlas.Pix[to:to+width*imageChannels], face.Pix[from:from+width*imageChannels])
	}
}
