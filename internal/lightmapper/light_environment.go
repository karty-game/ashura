//nolint:mnd,varnamelen // Latitude/longitude Jacobians and the fixed CDF mixture retain their mathematical notation.
package lightmapper

import (
	"math"
	"sort"
)

// Two-level float32 CDFs retain positive support even for extreme HDR maps.
// An explicit one-eighth uniform-index mixture and admitted8192 dimensions
// keep every quantized row/column interval well above a float32 ULP.
func (p *PreparedScene) buildEnvironment() {
	if p.source.Environment.Texture == 0 {
		return
	}

	t := p.source.Textures[p.source.Environment.Texture-1]
	p.environmentRows = make([]float32, int(t.Height)+1)
	p.environmentColumns = make([]float32, int(t.Height)*(int(t.Width)+1))

	weights := make([]float64, t.Height)
	for y := range t.Height {
		column := p.environmentColumns[int(y)*(int(t.Width)+1) : int(y+1)*(int(t.Width)+1)]

		sum := float64(0)
		for x := range t.Width {
			sum += float64(luminance(t.Pixels[y*t.Width+x]))
		}

		acc := float64(0)

		for x := range t.Width {
			importance := float64(1) / float64(t.Width)
			if sum > 0 {
				importance = float64(luminance(t.Pixels[y*t.Width+x])) / sum
			}

			acc += .875*importance + .125/float64(t.Width)
			column[x+1] = float32(acc)
		}

		column[t.Width] = 1
		weights[y] = sum * (math.Cos(math.Pi*float64(y)/float64(t.Height)) - math.Cos(math.Pi*float64(y+1)/float64(t.Height)))
	}

	total := float64(0)
	for _, w := range weights {
		total += w
	}

	acc := float64(0)

	for y, w := range weights {
		importance := float64(1) / float64(t.Height)
		if total > 0 {
			importance = w / total
		}

		acc += .875*importance + .125/float64(t.Height)
		p.environmentRows[y+1] = float32(acc)
	}

	p.environmentRows[t.Height] = 1
}

func (p *PreparedScene) EnvironmentRadiance(direction Vec3) Vec3 {
	e := p.source.Environment

	var value Vec3

	if e.Texture > 0 {
		u := float32(math.Atan2(float64(direction[2]), float64(direction[0]))/(2*math.Pi) + .5)
		v := float32(environmentTheta(direction) / math.Pi)
		value = sampleTextureMode(p.source.Textures[e.Texture-1], Vec2{u, v}, true)
	} else {
		value = mixVec(e.Ground, e.Sky, max(0, min(1, direction[1]*.5+.5)))
	}

	if e.LegacyIrradiance {
		value = Scale(value, 1/lightPi)
	}

	return value
}

//nolint:gosec // The selected CDF index is bounded by an admitted texture dimension of at most8192.
func sampleEnvironmentCDF(cdf []float32, u float32) (uint32, float32, float32) {
	i := min(sort.Search(len(cdf)-1, func(i int) bool { return cdf[i+1] > u }), len(cdf)-2)
	mass := cdf[i+1] - cdf[i]

	return uint32(i), (u - cdf[i]) / mass, mass
}

func (p *PreparedScene) SampleEnvironment(u, v float32) (Vec3, float32) {
	if len(p.environmentRows) == 0 {
		return p.sampleUniformSphere(u, v)
	}

	t := p.source.Textures[p.source.Environment.Texture-1]
	y, fy, py := sampleEnvironmentCDF(p.environmentRows, u)
	columns := p.environmentColumns[int(y)*(int(t.Width)+1) : int(y+1)*(int(t.Width)+1)]
	x, fx, px := sampleEnvironmentCDF(columns, v)
	theta0, theta1 := math.Pi*float64(y)/float64(t.Height), math.Pi*float64(y+1)/float64(t.Height)
	deltaCos := environmentDeltaCos(y, t.Height)

	var cy, radius float64

	if theta0+theta1 <= math.Pi {
		v := 2*math.Pow(math.Sin(theta0*.5), 2) + deltaCos*float64(fy)
		cy = 1 - v
		radius = math.Sqrt(max(0, v*(2-v)))
	} else {
		v := 2*math.Pow(math.Cos(theta1*.5), 2) + deltaCos*(1-float64(fy))
		cy = -1 + v
		radius = math.Sqrt(max(0, v*(2-v)))
	}

	phi := 2 * math.Pi * ((float64(x)+float64(fx))/float64(t.Width) - .5)
	solid := 2 * math.Pi / float64(t.Width) * deltaCos

	return Vec3{float32(radius * math.Cos(phi)), float32(cy), float32(radius * math.Sin(phi))}, float32(float64(px*py) / solid)
}

func (p *PreparedScene) EnvironmentPDF(direction Vec3) float32 {
	if len(p.environmentRows) == 0 {
		return 1 / (4 * lightPi)
	}

	t := p.source.Textures[p.source.Environment.Texture-1]
	u := math.Atan2(float64(direction[2]), float64(direction[0]))/(2*math.Pi) + .5
	v := environmentTheta(direction) / math.Pi
	x := min(t.Width-1, uint32(u*float64(t.Width)))
	y := min(t.Height-1, uint32(v*float64(t.Height)))
	columns := p.environmentColumns[int(y)*(int(t.Width)+1) : int(y+1)*(int(t.Width)+1)]
	mass := (p.environmentRows[y+1] - p.environmentRows[y]) * (columns[x+1] - columns[x])
	solid := 2 * math.Pi / float64(t.Width) * environmentDeltaCos(y, t.Height)

	return float32(float64(mass) / solid)
}

func environmentTheta(direction Vec3) float64 {
	return math.Atan2(math.Hypot(float64(direction[0]), float64(direction[2])), float64(direction[1]))
}
func environmentDeltaCos(y, height uint32) float64 {
	return 2 * math.Sin(math.Pi*(float64(y)+.5)/float64(height)) * math.Sin(math.Pi*.5/float64(height))
}
