package lightmapper

import (
	"math"
	"testing"
)

func TestCalibratedProjectionPreservesFlatAndKnownFit(t *testing.T) {
	t.Parallel()

	hessian := CalibrationHessian()

	for _, coefficients := range [][3]float64{{.2, .6, 1.3}, {0, .4, 2}, {0, 0, 3}, {0, 0, 0}} {
		var moments Moments

		for channel := range 3 {
			scale := float64(channel + 1)
			moments[0][channel] = float32((coefficients[0] + coefficients[1] + coefficients[2]) * scale / 3)

			for i := range 3 {
				for j := range 3 {
					moments[i+1][channel] += float32(hessian[i][j] * coefficients[j] * scale)
				}
			}
		}

		fitted, err := ProjectRNM(moments)
		if err != nil {
			t.Fatal(err)
		}

		for channel := range 3 {
			var flat float32

			for i := range 3 {
				if fitted[i][channel] < 0 || math.Abs(float64(fitted[i][channel])-coefficients[i]*float64(channel+1)) > 1e-5 {
					t.Fatal(coefficients, fitted)
				}

				flat += fitted[i][channel] / 3
			}

			if math.Abs(float64(flat-moments[0][channel])) > 1e-6 {
				t.Fatal(flat, moments[0])
			}
		}
	}
}

func TestCalibrationKernelMatchesFullProbeRHS(t *testing.T) {
	t.Parallel()

	direction := Normalize(Vec3{.7, -.4, .1})
	kernels := CalibrationKernels(direction)

	var expected [4]float32

	expected[0] = max(float32(0), direction[2]) / math.Pi

	probes := CalibrationProbes()
	for _, probe := range probes[1:] {
		response := max(float32(0), Dot(probe.Normal, direction)) / math.Pi

		weights := CalibrationWeights(probe.Normal)
		for i := range 3 {
			expected[i+1] += probe.Weight * weights[i] * response
		}
	}

	if expected != kernels {
		t.Fatal(expected, kernels)
	}
}

func TestHDRCalibratedProjectionKeepsAllThreeCoefficients(t *testing.T) {
	t.Parallel()

	hessian := CalibrationHessian()

	var moments Moments
	for channel := range 3 {
		moments[0][channel] = 1e20

		for i := range 3 {
			for j := range 3 {
				moments[i+1][channel] += float32(hessian[i][j] * 1e20)
			}
		}
	}

	coefficients, err := ProjectRNM(moments)
	if err != nil {
		t.Fatal(err)
	}

	for _, coefficient := range coefficients {
		for _, value := range coefficient {
			if math.Abs(float64(value)/1e20-1) > 1e-5 {
				t.Fatal(coefficients)
			}
		}
	}
}
