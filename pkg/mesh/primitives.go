package mesh

import (
	"math"

	engine "github.com/karty-game/karty-webgpu/pkg/engine"
)

const (
	fullTurn                   = 2.0
	capsuleRingDivisor         = 4
	centeredUV         float32 = 0.5
)

type profileRing struct {
	radius    float32
	y         float32
	normalRad float32
	normalY   float32
}

func boxData(width, height, depth float32) Data {
	halfWidth, verticalHalf, halfDepth := width/half, height/half, depth/half
	corners := [8]engine.Vector3{
		{X: -halfWidth, Y: -verticalHalf, Z: -halfDepth},
		{X: halfWidth, Y: -verticalHalf, Z: -halfDepth},
		{X: halfWidth, Y: verticalHalf, Z: -halfDepth},
		{X: -halfWidth, Y: verticalHalf, Z: -halfDepth},
		{X: -halfWidth, Y: -verticalHalf, Z: halfDepth},
		{X: halfWidth, Y: -verticalHalf, Z: halfDepth},
		{X: halfWidth, Y: verticalHalf, Z: halfDepth},
		{X: -halfWidth, Y: verticalHalf, Z: halfDepth},
	}
	faces := [6][4]uint32{{4, 5, 6, 7}, {1, 0, 3, 2}, {5, 1, 2, 6}, {0, 4, 7, 3}, {7, 6, 2, 3}, {0, 1, 5, 4}}
	normals := [6]engine.Vector3{{Z: 1}, {Z: -1}, {X: 1}, {X: -1}, {Y: 1}, {Y: -1}}
	faceUV := [4]engine.Vector2{{X: 0, Y: 1}, {X: 1, Y: 1}, {X: 1, Y: 0}, {X: 0, Y: 0}}
	data := Data{
		Positions: make([]engine.Vector3, 0, 24),
		Normals:   make([]engine.Vector3, 0, 24),
		UVs:       make([]engine.Vector2, 0, 24),
		Indices:   make([]uint32, 0, 36),
	}

	var base uint32

	for faceIndex, face := range faces {
		for _, cornerIndex := range face {
			data.Positions = append(data.Positions, corners[cornerIndex])
			data.Normals = append(data.Normals, normals[faceIndex])
		}

		data.UVs = append(data.UVs, faceUV[:]...)
		data.Indices = append(data.Indices, base, base+1, base+2, base, base+2, base+3)
		base += 4
	}

	return data
}

func quad(name string, width, height float32) Mesh {
	halfWidth, halfHeight := width/half, height/half
	positions := []engine.Vector3{
		{X: -halfWidth, Y: 0, Z: -halfHeight},
		{X: halfWidth, Y: 0, Z: -halfHeight},
		{X: halfWidth, Y: 0, Z: halfHeight},
		{X: -halfWidth, Y: 0, Z: halfHeight},
	}
	normal := engine.Vector3{Y: 1}

	return Mesh{
		Name: name,
		Data: Data{
			Positions: positions,
			Indices:   []uint32{0, 2, 1, 0, 3, 2},
			Normals:   []engine.Vector3{normal, normal, normal, normal},
			UVs:       []engine.Vector2{{X: 0, Y: 1}, {X: 1, Y: 1}, {X: 1, Y: 0}, {X: 0, Y: 0}},
		},
	}
}

func billboard(name string, width, height float32) Mesh {
	halfWidth, halfHeight := width/half, height/half
	positions := []engine.Vector3{
		{X: -halfWidth, Y: -halfHeight},
		{X: halfWidth, Y: -halfHeight},
		{X: halfWidth, Y: halfHeight},
		{X: -halfWidth, Y: halfHeight},
	}
	normal := engine.Vector3{Z: 1}

	return Mesh{
		Name:      name,
		Billboard: true,
		Data: Data{
			Positions: positions,
			Indices:   []uint32{0, 1, 2, 0, 2, 3},
			Normals:   []engine.Vector3{normal, normal, normal, normal},
			UVs:       []engine.Vector2{{X: 0, Y: 1}, {X: 1, Y: 1}, {X: 1, Y: 0}, {X: 0, Y: 0}},
		},
	}
}

func cylinder(name string, height, radius float32, segments uint32) Mesh {
	data := Data{}
	halfHeight := height / half

	// Side vertices are duplicated at the UV seam and from each cap so the
	// side and cap normals remain independent.
	for segment := uint32(0); segment <= segments; segment++ {
		angle := fullTurn * math.Pi * float64(segment) / float64(segments)
		radialX := float32(math.Cos(angle)) * radius
		radialZ := float32(math.Sin(angle)) * radius
		normal := engine.Vector3{X: float32(math.Cos(angle)), Z: float32(math.Sin(angle))}
		horizontalUV := float32(segment) / float32(segments)

		data.Positions = append(data.Positions,
			engine.Vector3{X: radialX, Y: -halfHeight, Z: radialZ},
			engine.Vector3{X: radialX, Y: halfHeight, Z: radialZ},
		)
		data.Normals = append(data.Normals, normal, normal)
		data.UVs = append(data.UVs,
			engine.Vector2{X: horizontalUV, Y: 1},
			engine.Vector2{X: horizontalUV, Y: 0},
		)
	}

	for segment := range segments {
		base := segment * half
		data.Indices = append(data.Indices, base, base+1, base+2, base+1, base+3, base+2)
	}

	bottomCapCenter := (segments + 1) * half
	appendCylinderCap(&data, bottomCapCenter, radius, -halfHeight, segments, false)
	topCapCenter := bottomCapCenter + segments + half
	appendCylinderCap(&data, topCapCenter, radius, halfHeight, segments, true)

	return Mesh{Name: name, Data: data}
}

func appendCylinderCap(data *Data, center uint32, radius, centerY float32, segments uint32, top bool) {
	normal := engine.Vector3{Y: -1}
	if top {
		normal.Y = 1
	}

	data.Positions = append(data.Positions, engine.Vector3{Y: centerY})
	data.Normals = append(data.Normals, normal)
	data.UVs = append(data.UVs, engine.Vector2{X: centeredUV, Y: centeredUV})

	for segment := uint32(0); segment <= segments; segment++ {
		angle := fullTurn * math.Pi * float64(segment) / float64(segments)
		radialX := float32(math.Cos(angle)) * radius
		radialZ := float32(math.Sin(angle)) * radius
		data.Positions = append(data.Positions, engine.Vector3{X: radialX, Y: centerY, Z: radialZ})
		data.Normals = append(data.Normals, normal)
		data.UVs = append(data.UVs, engine.Vector2{X: centeredUV + radialX/(half*radius), Y: centeredUV + radialZ/(half*radius)})
	}

	for segment := range segments {
		first, second := center+1+segment, center+half+segment
		if top {
			data.Indices = append(data.Indices, center, second, first)
		} else {
			data.Indices = append(data.Indices, center, first, second)
		}
	}
}

func capsule(name string, height, radius float32, segments uint32) Mesh {
	coreHalf := height/half - radius
	hemisphereRings := max(segments/capsuleRingDivisor, 2)
	profile := makeCapsuleProfile(coreHalf, radius, hemisphereRings)
	data := Data{}

	for row, ring := range profile {
		appendCapsuleRow(&data, row, ring, profile, segments)
	}

	appendCapsuleIndices(&data, len(profile), segments)

	return Mesh{Name: name, Data: data}
}

func makeCapsuleProfile(coreHalf, radius float32, hemisphereRings uint32) []profileRing {
	profile := make([]profileRing, 0, int(hemisphereRings*2+2))

	for ring := uint32(0); ring <= hemisphereRings; ring++ {
		angle := -math.Pi/half + float64(ring)/float64(hemisphereRings)*math.Pi/half
		profile = append(profile, profileRing{
			radius:    float32(math.Cos(angle)) * radius,
			y:         -coreHalf + float32(math.Sin(angle))*radius,
			normalRad: float32(math.Cos(angle)),
			normalY:   float32(math.Sin(angle)),
		})
	}

	if coreHalf > 0 {
		profile = append(profile, profileRing{radius: radius, y: coreHalf, normalRad: 1})
	}

	for ring := uint32(1); ring <= hemisphereRings; ring++ {
		angle := float64(ring) / float64(hemisphereRings) * math.Pi / half
		profile = append(profile, profileRing{
			radius:    float32(math.Cos(angle)) * radius,
			y:         coreHalf + float32(math.Sin(angle))*radius,
			normalRad: float32(math.Cos(angle)),
			normalY:   float32(math.Sin(angle)),
		})
	}

	return profile
}

func appendCapsuleRow(data *Data, row int, ring profileRing, profile []profileRing, segments uint32) {
	for segment := uint32(0); segment <= segments; segment++ {
		horizontalUV := float32(segment) / float32(segments)
		angle := fullTurn * math.Pi * float64(horizontalUV)
		xDirection := float32(math.Cos(angle))
		zDirection := float32(math.Sin(angle))
		data.Positions = append(data.Positions, engine.Vector3{
			X: ring.radius * xDirection,
			Y: ring.y,
			Z: ring.radius * zDirection,
		})
		data.Normals = append(data.Normals, engine.Vector3{
			X: ring.normalRad * xDirection,
			Y: ring.normalY,
			Z: ring.normalRad * zDirection,
		})
		data.UVs = append(data.UVs, engine.Vector2{X: horizontalUV, Y: float32(row) / float32(len(profile)-1)})
	}
}

func appendCapsuleIndices(data *Data, profileLength int, segments uint32) {
	stride := segments + 1

	var rowBase uint32

	for range profileLength - 1 {
		for segment := range segments {
			first := rowBase + segment
			second := first + stride
			data.Indices = append(data.Indices, first, second, first+1, first+1, second, second+1)
		}

		rowBase += stride
	}
}
