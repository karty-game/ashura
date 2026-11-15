//go:build !js

//nolint:mnd,varnamelen // Tessellation, spherical coordinates and source-art ripples are fixed authored geometry.
package impostorfixture

import "math"

func (target *part) vertex(position, normal vector) {
	target.positions = append(target.positions, float32(position.x), float32(position.y), float32(position.z))
	target.normals = append(target.normals, float32(normal.x), float32(normal.y), float32(normal.z))
}

func (target *part) tube(path []vector, radii []float64, sides int, phase float64) {
	start := len(target.positions) / 3
	for ring, center := range path {
		tangent := path[min(ring+1, len(path)-1)].subtract(path[max(ring-1, 0)]).unit()

		helper := vector{0, 1, 0}
		if math.Abs(tangent.y) > .9 {
			helper = vector{1, 0, 0}
		}

		right := helper.cross(tangent).unit()
		up := tangent.cross(right)

		for side := range sides + 1 {
			angle := float64(side) * 2 * math.Pi / float64(sides)
			normal := right.scale(math.Cos(angle)).add(up.scale(math.Sin(angle)))
			rib := 1 + .055*math.Cos(angle*7+phase+float64(ring)*.15)
			target.vertex(center.add(normal.scale(radii[ring]*rib)), normal)
		}
	}

	for ring := range len(path) - 1 {
		for side := range sides {
			a := start + ring*(sides+1) + side
			b, c, d := a+1, a+sides+1, a+sides+2
			target.triangle(a, b, c)
			target.triangle(b, d, c)
		}
	}

	target.cap(path[0], path[0].subtract(path[1]).unit(), radii[0], sides, phase, true)
	last := len(path) - 1
	target.cap(path[last], path[last].subtract(path[last-1]).unit(), radii[last], sides, phase+float64(last)*.15, false)
}

func (target *part) cap(center, normal vector, radius float64, sides int, phase float64, start bool) {
	axis := normal
	if start {
		axis = normal.scale(-1)
	}

	helper := vector{0, 1, 0}
	if math.Abs(axis.y) > .9 {
		helper = vector{1, 0, 0}
	}

	right := helper.cross(axis).unit()
	up := axis.cross(right)
	base := len(target.positions) / 3
	target.vertex(center, normal)

	for side := range sides {
		angle := float64(side) * 2 * math.Pi / float64(sides)
		direction := right.scale(math.Cos(angle)).add(up.scale(math.Sin(angle)))
		rib := 1 + .055*math.Cos(angle*7+phase)
		target.vertex(center.add(direction.scale(radius*rib)), normal)
	}

	for side := range sides {
		first, second := base+1+side, base+1+(side+1)%sides
		if start {
			target.triangle(base, second, first)
		} else {
			target.triangle(base, first, second)
		}
	}
}

func (target *part) ellipsoid(center, radius vector, stacks, sides int, phase float64) {
	start := len(target.positions) / 3

	for stack := range stacks + 1 {
		latitude := float64(stack) * math.Pi / float64(stacks)
		for side := range sides + 1 {
			longitude := float64(side) * 2 * math.Pi / float64(sides)
			direction := vector{math.Sin(latitude) * math.Cos(longitude), math.Cos(latitude), math.Sin(latitude) * math.Sin(longitude)}
			rib := 1 + .055*math.Sin(longitude*3+phase)*math.Sin(latitude*4)
			position := vector{direction.x * radius.x * rib, direction.y * radius.y * rib, direction.z * radius.z * rib}
			normal := vector{direction.x / radius.x, direction.y / radius.y, direction.z / radius.z}.unit()
			target.vertex(center.add(position), normal)
		}
	}

	for stack := range stacks {
		for side := range sides {
			a := start + stack*(sides+1) + side
			b, c, d := a+1, a+sides+1, a+sides+2

			if stack > 0 {
				target.triangle(a, b, c)
			}

			if stack < stacks-1 {
				target.triangle(b, d, c)
			}
		}
	}
}

//nolint:gosec // All indices originate from the fixture's bounded authored loops.
func (target *part) triangle(a, b, c int) {
	target.indices = append(target.indices, uint32(a), uint32(b), uint32(c))
}
