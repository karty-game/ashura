//go:build !js

// Package impostorfixture authors the maintained, opaque GLB tree used by
// impostor regression tests. It is build-only and uses no runtime renderer packages.
//
//nolint:mnd // Authored coordinates, tessellation and shape variation describe the procedural source artwork.
package impostorfixture

import "math"

// Stats describes the actual authored source geometry and its bounds.
type Stats struct {
	Triangles, Vertices, Primitives int
	Min, Max                        [3]float32
}

type vector struct{ x, y, z float64 }

type part struct {
	positions, normals []float32
	indices            []uint32
}

// Tree produces a deterministic, asymmetric tree with roots, twisted trunk,
// branching limbs and solid foliage clusters, merged into four material parts.
func Tree() ([]byte, Stats, error) {
	parts := make([]part, 4)
	trunk := make([]vector, 23)

	radii := make([]float64, len(trunk))
	for index := range trunk {
		t := float64(index) / float64(len(trunk)-1)
		trunk[index] = vector{.17 * math.Sin(t*5.2), t * 4.4, .12 * math.Sin(t*7.1)}
		radii[index] = .37*(1-t) + .09
	}

	parts[0].tube(trunk, radii, 24, .2)
	addRoots(&parts[0])

	for branch := range 8 {
		addBranch(parts, branch)
	}

	return encode(parts)
}

func addRoots(target *part) {
	for root := range 9 {
		angle := float64(root)*2*math.Pi/9 + .2
		path := make([]vector, 7)
		radii := make([]float64, len(path))

		for index := range path {
			t := float64(index) / float64(len(path)-1)
			distance := .08 + t*(1.1+.18*math.Sin(float64(root)*2.1))
			path[index] = vector{math.Cos(angle) * distance, .08 + .3*(1-t)*(1-t), math.Sin(angle) * distance}
			radii[index] = .19*(1-t) + .025
		}

		target.tube(path, radii, 14, angle)
	}
}

func addBranch(parts []part, branch int) {
	angle := float64(branch)*2*math.Pi/8 + .18
	distance := 1.55 + .25*math.Sin(float64(branch)*2.3)
	end := vector{math.Cos(angle) * distance, 3.7 + .45*math.Cos(float64(branch)*1.7), math.Sin(angle) * distance}
	start := vector{.06, 1.65 + float64(branch)*.18, 0}
	path := make([]vector, 10)
	radii := make([]float64, len(path))

	for index := range path {
		t := float64(index) / float64(len(path)-1)
		path[index] = start.scale(1 - t).add(end.scale(t))
		path[index].y -= .32 * math.Sin(t*math.Pi)
		path[index].z += .14 * math.Sin(t*math.Pi*2+angle) * math.Sin(t*math.Pi)
		radii[index] = .19*(1-t) + .04
	}

	parts[branch%2].tube(path, radii, 16, angle)

	for twig := range 3 {
		addTwig(&parts[branch%2], path[5+twig], end, angle, twig)
	}

	for cluster := range 8 {
		clusterAngle := angle*.6 + float64(cluster)*2*math.Pi/8
		center := end.add(vector{.48 * math.Cos(clusterAngle), .3*math.Sin(float64(cluster)*2.2) + .25, .46 * math.Sin(clusterAngle)})
		size := vector{.57 + .11*math.Sin(float64(cluster+branch)*1.9), .48 + .1*math.Cos(float64(cluster)*1.3), .55}
		parts[2+(cluster+branch)%2].ellipsoid(center, size, 10, 16, float64(branch+cluster))
	}
}

func addTwig(target *part, start, end vector, angle float64, twig int) {
	direction := angle + float64(twig-1)*.55
	finish := end.add(vector{.6 * math.Cos(direction), .25 + float64(twig)*.15, .6 * math.Sin(direction)})
	path := make([]vector, 5)

	radii := make([]float64, len(path))
	for index := range path {
		t := float64(index) / float64(len(path)-1)
		path[index] = start.scale(1 - t).add(finish.scale(t))
		radii[index] = .08*(1-t) + .018
	}

	target.tube(path, radii, 12, direction)
}

func (value vector) add(other vector) vector {
	return vector{value.x + other.x, value.y + other.y, value.z + other.z}
}
func (value vector) subtract(other vector) vector {
	return vector{value.x - other.x, value.y - other.y, value.z - other.z}
}
func (value vector) scale(factor float64) vector {
	return vector{value.x * factor, value.y * factor, value.z * factor}
}
func (value vector) dot(other vector) float64 {
	return value.x*other.x + value.y*other.y + value.z*other.z
}
func (value vector) unit() vector { return value.scale(1 / math.Sqrt(value.dot(value))) }
func (value vector) cross(other vector) vector {
	return vector{value.y*other.z - value.z*other.y, value.z*other.x - value.x*other.z, value.x*other.y - value.y*other.x}
}
