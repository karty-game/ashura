//go:build ignore

package main

import (
	"slices"

	"github.com/karty-game/ashura/pkg/assetbuild"
	"github.com/karty-game/ashura/pkg/collision"
	collisionbuild "github.com/karty-game/ashura/pkg/collision/build"
	decalbuild "github.com/karty-game/ashura/pkg/decal/build"
	"github.com/karty-game/ashura/pkg/engine"
	"github.com/karty-game/ashura/samples/decals/internal/room"
)

const (
	atlas      = "atlas/switchyard"
	floorStamp = "stamp-albedo"
)

func main() {
	assetbuild.Main(func(assetbuild.Environment) assetbuild.Config {
		build := []assetbuild.Plugin{decalbuild.Projectors(decalbuild.ProjectorOptions{
			Name: "YardDetails", Atlas: atlas, Marks: authoredMarks(),
		})}
		for _, panel := range room.Panels() {
			for _, paint := range room.Paints() {
				build = append(build, decalbuild.Projectors(decalbuild.ProjectorOptions{
					Name: panel.Name + paint.Name, Atlas: atlas, Marks: sprayMarks(panel, paint),
				}))
			}
		}
		build = append(build, assetbuild.KTX2(), assetbuild.GoCatalog())

		boxes := []collision.Bounds{room.GroundBounds()}
		for _, surface := range room.Surfaces() {
			boxes = append(boxes, surface.Bounds)
		}

		return assetbuild.Config{
			Output: "generated/assets",
			Pipelines: []assetbuild.Pipeline{
				{
					Name: "yardTextures", Input: []string{"assets/yard-*-albedo.png", "assets/fox-mural-albedo.png",
						"assets/stripes-albedo.png", "assets/" + floorStamp + ".png"},
					Import: []assetbuild.Plugin{assetbuild.Images()},
					Process: []assetbuild.Plugin{
						bakeFloor{},
						assetbuild.Resize(assetbuild.ResizeOptions{MaxDimension: 800, Include: []string{"fox-mural-albedo"}}),
						assetbuild.Atlas(assetbuild.AtlasOptions{
							Name: "switchyard",
							Include: []string{
								"fox-mural-albedo", "yard-dab-albedo", "yard-grime-albedo", "yard-rivet-albedo",
								"yard-notice-albedo", "yard-fleck-albedo", "yard-sign-albedo", "yard-tag-albedo",
								"yard-freight-albedo", "yard-arrow-albedo", "stripes-albedo",
							},
							MaxSize: 1024, Padding: 8, MipLevels: 4, Clamp: true,
						}),
						assetbuild.Mipmaps(),
					},
					Output: build,
				},
				{
					Name: "yardCollision", Output: []assetbuild.Plugin{collisionbuild.Boxes("Yard", boxes...)},
				},
			},
		}
	})
}

type bakeFloor struct{}

func (bakeFloor) Name() string { return "bake-yard-floor" }

func (bakeFloor) Plan(plan *assetbuild.Plan) error {
	if err := decalbuild.BakeAlbedo(floorBake()).Plan(plan); err != nil {
		return err
	}

	// The stamp remains an input to the bake. Only the composited asphalt is used at runtime.
	plan.Resources = slices.DeleteFunc(plan.Resources, func(resource assetbuild.Resource) bool {
		return resource.ID == floorStamp
	})

	return nil
}

func floorBake() decalbuild.BakeOptions {
	geometry, transform := room.Floor()
	vertices := geometry.Data
	triangles := make([]decalbuild.Triangle, 0, len(vertices.Indices)/3)
	for first := 0; first < len(vertices.Indices); first += 3 {
		var triangle decalbuild.Triangle
		for corner, index := range vertices.Indices[first : first+3] {
			position := vertices.Positions[index]
			position.X += transform.Position.X
			position.Y += transform.Position.Y
			position.Z += transform.Position.Z
			triangle.Positions[corner], triangle.UVs[corner] = position, vertices.UVs[index]
		}
		triangles = append(triangles, triangle)
	}

	yellow := engine.Color{R: .97, G: .74, B: .27, A: .84}
	marks := []decalbuild.Mark{
		ground("yard-arrow-albedo", -5.7, 2.15, 2.55, 1.68, 0, yellow),
		ground("yard-arrow-albedo", .2, 2.65, 2.55, 1.68, 0, yellow),
		ground("yard-arrow-albedo", -5.7, 6.95, 2.55, 1.68, 0, yellow),
		ground(floorStamp, -7.75, 1.1, 2.15, 2.15, .11, engine.Color{R: .24, G: .63, B: .61, A: .83}),
	}
	return decalbuild.BakeOptions{
		Receiver: "yard-asphalt-albedo", Layers: room.FloorLayer, Triangles: triangles, Marks: marks,
	}
}

func authoredMarks() []decalbuild.Mark {
	white := engine.ColorWhite()
	marks := []decalbuild.Mark{
		vertical("fox-mural", -.57, 2.58, room.WallFront, 9.7, 4.4, room.WallLayer, white, 12),
		vertical("yard-sign", -9.53, 3.89, room.WallFront, 4.7, 1.35, room.WallLayer, white, 15),
		vertical("yard-freight", 8.79, 2.6, room.CarFront, 3.25, 1.48, room.CarLayer, white, 15),
		vertical("stripes", 9.43, .57, room.WallFront, 5.25, .52, room.WallLayer, white, 15),
		vertical("stripes", -9.58, .43, room.WallFront, 4.85, .36, room.WallLayer, white, 15),
	}
	marks = append(marks, wearOnWall()...)
	marks = append(marks, wallStickers()...)
	marks = append(marks, carDetails()...)
	marks = append(marks, groundDetails()...)
	return marks
}

func wearOnWall() []decalbuild.Mark {
	var marks []decalbuild.Mark
	for index := range 196 {
		lane, column := index/49, index%49
		jitter := noise(index, 2)
		x := -12.48 + float32(column)*.512 + (jitter-.5)*.42
		y := .43 + float32(lane)*1.18 + (noise(index, 11)-.5)*.90
		tint := engine.Color{R: .90, G: .80, B: .67, A: .59 + .40*noise(index, 15)}
		mark := vertical("yard-grime", x, y, room.WallFront, .28+jitter*.76, .34+noise(index, 5)*1.24, room.WallLayer, tint, 2)
		mark.Angle = (noise(index, 19) - .5) * .58
		marks = append(marks, mark)
	}
	for index := range 88 {
		x := -12.65 + float32(index%44)*.587
		y := []float32{.36, 4.92}[index/44]
		marks = append(marks, vertical("yard-rivet", x, y, room.WallFront, .13, .13, room.WallLayer, engine.ColorWhite(), 4))
	}
	return marks
}

func wallStickers() []decalbuild.Mark {
	var marks []decalbuild.Mark
	for index := range 24 {
		side, column := index/12, index%4
		left := []float32{-12.15, 6.72}[side]
		x := left + float32(column)*1.22 + (noise(index, 55)-.5)*.66
		y := .92 + float32(index/4%3)*.8 + (noise(index, 56)-.5)*.41
		image := []string{"yard-notice", "yard-tag", "yard-arrow", "yard-tag"}[index%4]
		size := []engine.Vector2{{X: .42, Y: .53}, {X: .9, Y: .38}, {X: .63, Y: .40}, {X: 1.23, Y: .51}}[index%4]
		tint := engine.Color{R: .86 + .12*noise(index, 21), G: .69 + .29*noise(index, 23), B: .64 + .34*noise(index, 17), A: 1}
		mark := vertical(image, x, y, room.WallFront, size.X, size.Y, room.WallLayer, tint, 8)
		mark.Angle = (noise(index, 6) - .5) * .33
		marks = append(marks, mark)
	}
	return marks
}

func carDetails() []decalbuild.Mark {
	var marks []decalbuild.Mark
	for index := range 80 {
		x := 5.9 + float32(index%40)*.14
		y := []float32{1.04, 3.39}[index/40]
		marks = append(marks, vertical("yard-rivet", x, y, room.CarFront, .10, .10, room.CarLayer, engine.ColorWhite(), 4))
	}
	for index := range 32 {
		x := 5.98 + noise(index, 68)*5.32
		y := 1.05 + noise(index, 78)*2.36
		tint := engine.Color{R: 1, G: .71, B: .41, A: .76}
		marks = append(marks, vertical("yard-grime", x, y, room.CarFront, .34, .46+noise(index, 12)*.66, room.CarLayer, tint, 3))
	}
	return marks
}

func groundDetails() []decalbuild.Mark {
	var marks []decalbuild.Mark
	for index := range 192 {
		x := -13.65 + float32(index%48)*.567 + (noise(index, 57)-.5)*.52
		z := -3.64 + float32(index/48)*3.17 + (noise(index, 58)-.5)*2.3
		tint := engine.Color{R: .95, G: .85, B: .59, A: .56 + .44*noise(index, 33)}
		image := []string{"yard-fleck-albedo", "yard-grime-albedo", "yard-fleck-albedo", "yard-grime-albedo"}[index%4]
		mark := ground(image, x, z, .32+noise(index, 70)*1.04, .24+noise(index, 24)*.63, noise(index, 79)*6, tint)
		mark.Image += ".png"
		marks = append(marks, mark)
	}
	for index := range 32 {
		x := -13.1 + float32(index)*.84
		mark := ground("stripes-albedo.png", x, 4.47, .47, .21, 0, engine.Color{R: 1, G: .85, B: .64, A: .63})
		marks = append(marks, mark)
	}
	return marks
}

func sprayMarks(panel room.Panel, paint room.Paint) []decalbuild.Mark {
	marks := make([]decalbuild.Mark, panel.Count())
	dx, dy := panel.Spacing()
	for index := range marks {
		pos := panel.Center(index)
		tint := paint.Color
		tint.A *= .87 + .13*noise(index, 84)
		size := 1.74 + .16*noise(index, 100)
		marks[index] = vertical("yard-dab", pos.X, pos.Y, pos.Z, dx*size, dy*size, panel.Layers, tint, 100)
		marks[index].Angle = noise(index, 5) * 4
	}
	return marks
}

func vertical(image string, x, y, z, width, height float32, layers uint32, tint engine.Color, order int32) decalbuild.Mark {
	return decalbuild.Mark{
		Image: image + "-albedo.png", Position: engine.Vector3{X: x, Y: y, Z: z}, Normal: engine.Vector3{Z: 1},
		Size: engine.Vector3{X: width, Y: height, Z: .075}, Tint: tint, ReceiverMask: layers, Order: order,
	}
}

func ground(image string, x, z, width, height, angle float32, tint engine.Color) decalbuild.Mark {
	return decalbuild.Mark{
		Image: image, Position: engine.Vector3{X: x, Z: z}, Normal: engine.Vector3Up(),
		Size: engine.Vector3{X: width, Y: height, Z: .09}, Angle: angle, Tint: tint, ReceiverMask: room.FloorLayer,
	}
}
func noise(index, salt int) float32 {
	value := uint32(index*218939 + salt*86699 + 13)
	value ^= value >> 16
	value *= 0x7feb352d
	value ^= value >> 15
	return float32(value&65535) / 65535
}
