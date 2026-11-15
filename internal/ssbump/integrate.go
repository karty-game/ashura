//go:build !js

//nolint:mnd,varnamelen // Fourier and finite-difference constants and indices follow the periodic Poisson equation.
package ssbump

import (
	"math"
	"math/cmplx"
)

// integrate solves the periodic Poisson equation for the least-squares surface
// whose forward differences best fit -Nx/Nz and -Ny/Nz. DC is fixed to zero.
func integrate(normals [][3]float64, size int) []float64 {
	gradient := make([][2]float64, len(normals))
	for i, n := range normals {
		z := math.Max(.05, n[2])
		gradient[i] = [2]float64{-n[0] / z, -n[1] / z}
	}

	spectrum := make([]complex128, len(normals))

	for y := range size {
		for x := range size {
			i := y*size + x
			left := y*size + ((x - 1) & (size - 1))
			above := ((y-1)&(size-1))*size + x
			spectrum[i] = complex(gradient[i][0]-gradient[left][0]+gradient[i][1]-gradient[above][1], 0)
		}
	}

	transform2D(spectrum, size, false)

	for y := range size {
		for x := range size {
			eigenvalue := 2*math.Cos(2*math.Pi*float64(x)/float64(size)) + 2*math.Cos(2*math.Pi*float64(y)/float64(size)) - 4

			i := y*size + x
			if i == 0 {
				spectrum[i] = 0
			} else {
				spectrum[i] /= complex(eigenvalue, 0)
			}
		}
	}

	transform2D(spectrum, size, true)

	heights := make([]float64, len(normals))
	for i, value := range spectrum {
		heights[i] = real(value)
	}

	return heights
}

func transform2D(data []complex128, size int, inverse bool) {
	column := make([]complex128, size)
	for y := range size {
		fft(data[y*size:(y+1)*size], inverse)
	}

	for x := range size {
		for y := range size {
			column[y] = data[y*size+x]
		}

		fft(column, inverse)

		for y := range size {
			data[y*size+x] = column[y]
		}
	}
}

func fft(data []complex128, inverse bool) {
	count := len(data)
	for i, j := 1, 0; i < count; i++ {
		bit := count >> 1
		for j&bit != 0 {
			j ^= bit
			bit >>= 1
		}

		j ^= bit
		if i < j {
			data[i], data[j] = data[j], data[i]
		}
	}

	for width := 2; width <= count; width *= 2 {
		angle := -2 * math.Pi / float64(width)
		if inverse {
			angle = -angle
		}

		root := cmplx.Exp(complex(0, angle))
		for start := 0; start < count; start += width {
			factor := complex(1, 0)
			for j := range width / 2 {
				even, odd := data[start+j], data[start+j+width/2]*factor
				data[start+j], data[start+j+width/2] = even+odd, even-odd
				factor *= root
			}
		}
	}

	if inverse {
		for i := range data {
			data[i] /= complex(float64(count), 0)
		}
	}
}
