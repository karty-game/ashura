package lightmapper

import (
	"errors"
	"math"
)

// CalibrationProfile identifies the directional projection, independently of a codec.
const CalibrationProfile = "rnm-flat-30x8-60x8-3to1@1"

// Moments contains the reference-normal response and three least-squares RHS values.
// Values are linear diffuse factors D=E/pi, with receiver reflectance excluded.
type Moments [4]Vec3

// CalibrationProbe is one local shading normal and its least-squares weight.
type CalibrationProbe struct {
	Normal Vec3
	Weight float32
}

var errProjection = errors.New("lightmapper: invalid directional projection statistics")

// CalibrationProbes returns the named profile. The flat probe is an equality constraint.
//
//nolint:mnd // The versioned calibration uses eight azimuths per ring, a 3:1 ring weight and pi/4 angular steps.
func CalibrationProbes() [17]CalibrationProbe {
	var result [17]CalibrationProbe

	result[0].Normal = Vec3{0, 0, 1}

	for ring, angle := range [2]float64{math.Pi / 6, math.Pi / 3} {
		weight := float32(3.0 / 8)
		if ring == 1 {
			weight = 1.0 / 8
		}

		for azimuth := range 8 {
			phi := float64(azimuth) * math.Pi / 4
			result[1+ring*8+azimuth] = CalibrationProbe{
				Normal: Vec3{float32(math.Sin(angle) * math.Cos(phi)), float32(math.Sin(angle) * math.Sin(phi)), float32(math.Cos(angle))},
				Weight: weight,
			}
		}
	}

	return result
}

// CalibrationWeights matches unoccluded SSBump preparation. Visibility multiplies
// these weights later; visibility-weighted coefficients must not be renormalized.
func CalibrationWeights(normal Vec3) Vec3 {
	basis := [3]Vec3{
		{.816496580927726, 0, .577350269189626},
		{-.408248290463863, .707106781186548, .577350269189626},
		{-.408248290463863, -.707106781186548, .577350269189626},
	}

	var (
		weights Vec3
		sum     float32
	)

	for i, b := range basis {
		p := max(float32(0), normal[0]*b[0]+normal[1]*b[1]+normal[2]*b[2])
		weights[i] = p * p
		sum += weights[i]
	}

	if sum > 0 {
		for i := range weights {
			weights[i] /= sum
		}
	}

	return weights
}

// CalibrationKernels converts one local incident direction into the four shared
// accumulation kernels. Multiply by incoming radiance and divide by its actual PDF.
func CalibrationKernels(direction Vec3) [4]float32 {
	result := [4]float32{max(float32(0), direction[2]) / math.Pi}

	probes := CalibrationProbes()
	for _, probe := range probes[1:] {
		cosine := max(float32(0), probe.Normal[0]*direction[0]+probe.Normal[1]*direction[1]+probe.Normal[2]*direction[2]) / math.Pi

		weights := CalibrationWeights(probe.Normal)
		for i := range 3 {
			result[i+1] += probe.Weight * weights[i] * cosine
		}
	}

	return result
}

// CalibrationHessian returns A^T W A for the fixed profile.
//
//nolint:varnamelen // H is the conventional three-by-three least-squares Hessian.
func CalibrationHessian() [3][3]float64 {
	var h [3][3]float64

	probes := CalibrationProbes()
	for _, probe := range probes[1:] {
		w := CalibrationWeights(probe.Normal)

		for i := range 3 {
			for j := range 3 {
				h[i][j] += float64(probe.Weight) * float64(w[i]) * float64(w[j])
			}
		}
	}

	return h
}

// ProjectRNM solves the three-variable nonnegative fit with exact flat-energy
// constraint. The same candidate enumeration is used by the compute resolve.
//
//nolint:varnamelen // H is the shared three-by-three calibration Hessian.
func ProjectRNM(moments Moments) ([3]Vec3, error) {
	var result [3]Vec3

	for _, value := range moments {
		for _, v := range value {
			if v < 0 || math.IsNaN(float64(v)) || math.IsInf(float64(v), 0) {
				return result, errProjection
			}
		}
	}

	h := CalibrationHessian()

	for channel := range 3 {
		flat := float64(moments[0][channel])
		g := [3]float64{float64(moments[1][channel]), float64(moments[2][channel]), float64(moments[3][channel])}

		value := projectChannel(flat, g, h)
		for i := range 3 {
			if !receiverFinite(float32(value[i])) {
				return [3]Vec3{}, ErrBounds
			}

			result[i][channel] = float32(value[i])
		}
	}

	return result, nil
}

//nolint:mnd,varnamelen // The constrained three-coefficient solve uses standard H/g and i/j matrix notation, with sum(C)=3*flat.
func projectChannel(flat float64, g [3]float64, h [3][3]float64) [3]float64 {
	sum := 3 * flat
	best := [3]float64{sum, 0, 0}
	score := projectionObjective(best, g, h)
	consider := func(candidate [3]float64) {
		for _, value := range candidate {
			if value < 0 || math.IsNaN(value) {
				return
			}
		}

		next := projectionObjective(candidate, g, h)
		if next < score {
			best, score = candidate, next
		}
	}
	consider([3]float64{0, sum, 0})
	consider([3]float64{0, 0, sum})

	for i := range 3 {
		for j := i + 1; j < 3; j++ {
			denominator := h[i][i] + h[j][j] - 2*h[i][j]
			x := max(float64(0), min(sum, (g[i]-g[j]-sum*(h[i][j]-h[j][j]))/denominator))

			var candidate [3]float64

			candidate[i], candidate[j] = x, sum-x
			consider(candidate)
		}
	}

	a := h[0][0] + h[2][2] - 2*h[0][2]
	b := h[0][1] - h[0][2] - h[1][2] + h[2][2]
	d := h[1][1] + h[2][2] - 2*h[1][2]
	r0 := g[0] - g[2] - sum*(h[0][2]-h[2][2])
	r1 := g[1] - g[2] - sum*(h[1][2]-h[2][2])
	determinant := a*d - b*b
	x, y := (d*r0-b*r1)/determinant, (a*r1-b*r0)/determinant
	consider([3]float64{x, y, sum - x - y})

	return best
}

//nolint:mnd // The least-squares objective is C^T H C minus twice g^T C.
func projectionObjective(value, g [3]float64, h [3][3]float64) float64 {
	var result float64
	for i := range 3 {
		result -= 2 * g[i] * value[i]
		for j := range 3 {
			result += value[i] * h[i][j] * value[j]
		}
	}

	return result
}
