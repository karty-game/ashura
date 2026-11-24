package main

import (
	"math"

	"github.com/karty-game/ashura/pkg/collision"
	"github.com/karty-game/ashura/pkg/engine"
	"github.com/karty-game/ashura/pkg/material"
	"github.com/karty-game/ashura/pkg/mesh"
	"github.com/karty-game/ashura/pkg/texture"
	"github.com/karty-game/ashura/pkg/texture/ktx2"
	"github.com/karty-game/ashura/samples/courtyard/generated/assets"
	"github.com/karty-game/ashura/samples/courtyard/layout"
)

const (
	wallTileSize          = 2.3
	floorTileSize         = 1.6
	matte                 = .84
	gateReveal    float32 = .016
	headerBottom          = 2.28
	headerTop             = 2.48
	jambWidth             = .18
	jambDepth             = .82
	headerDepth           = .86
)

type courtyardArt struct {
	floor, brick, concrete         *texture.Texture
	copperGate, violetGate, panel  *texture.Texture
	cratePanel, crateFrame, hazard *texture.Texture
	arrow, waypoint                *texture.Texture
}

func loadCourtyardArt(app *engine.App) (courtyardArt, error) {
	var art courtyardArt
	for _, entry := range [...]struct {
		asset  ktx2.Asset
		target **texture.Texture
	}{
		{assets.FloorDarkChecker(), &art.floor},
		{assets.WallWhiteBrick(), &art.brick},
		{assets.WallBlueConcrete(), &art.concrete},
		{assets.WallOrangePanel(), &art.copperGate},
		{assets.WallPurplePanel(), &art.violetGate},
		{assets.WallWhitePanel(), &art.panel},
		{assets.CratePanel(), &art.cratePanel},
		{assets.CrateFrame(), &art.crateFrame},
		{assets.TrimHazard(), &art.hazard},
		{assets.DecalIconArrow(), &art.arrow},
		{assets.DecalIconWaypoint(), &art.waypoint},
	} {
		image, err := entry.asset.Load(app)
		if err != nil {
			return courtyardArt{}, err
		}

		*entry.target = image
	}

	return art, nil
}

func albedo(image *texture.Texture, color engine.Color) material.Material {
	surface := material.DefaultStandard()
	surface.Color, surface.AlbedoTexture, surface.Roughness = color, image, matte

	return surface
}

func (art courtyardArt) buildYards(world *engine.World) {
	floor := layout.Floor()
	tiles := albedo(art.floor, engine.ColorWhite())
	tiles.UVScale = engine.Vector2{
		X: (floor.Max.X - floor.Min.X) / floorTileSize, Y: (floor.Max.Z - floor.Min.Z) / floorTileSize,
	}
	addBlock(world, "checker floor", floor, tiles)

	for _, wall := range layout.Perimeter() {
		addWall(world, "white brick", wall, art.brick)
	}

	for _, wall := range layout.Dividers() {
		addWall(world, "blue concrete", wall, art.concrete)
	}

	cube := mesh.NewBox("crate piece", mesh.BoxOptions{})
	for _, box := range layout.Crates() {
		art.addCrate(world, &cube, box)
	}
}

func addWall(world *engine.World, name string, bounds collision.Bounds, image *texture.Texture) {
	width := max(bounds.Max.X-bounds.Min.X, bounds.Max.Z-bounds.Min.Z)
	surface := albedo(image, engine.ColorWhite())
	surface.UVScale = engine.Vector2{X: width / wallTileSize, Y: (bounds.Max.Y - bounds.Min.Y) / wallTileSize}
	addBlock(world, name, bounds, surface)
}

func (art courtyardArt) addCrate(world *engine.World, cube *mesh.Mesh, box collision.Bounds) {
	const (
		rim   = .12
		inset = .04
	)

	minimum, maximum := box.Min, box.Max
	panel := collision.Bounds{
		Min: engine.Vector3{X: minimum.X + inset, Y: minimum.Y + rim, Z: minimum.Z + inset},
		Max: engine.Vector3{X: maximum.X - inset, Y: maximum.Y - rim, Z: maximum.Z - inset},
	}
	bottom, top := box, box
	bottom.Max.Y, top.Min.Y = minimum.Y+rim, maximum.Y-rim

	addCratePiece(world, cube, "crate boards", panel, albedo(art.cratePanel, engine.ColorWhite()))

	frame := albedo(art.crateFrame, engine.ColorWhite())
	addCratePiece(world, cube, "crate base", bottom, frame)
	addCratePiece(world, cube, "crate rim", top, frame)

	for _, x := range [...]float32{minimum.X, maximum.X - rim} {
		for _, z := range [...]float32{minimum.Z, maximum.Z - rim} {
			addCratePiece(world, cube, "crate corner", collision.Bounds{
				Min: engine.Vector3{X: x, Y: minimum.Y + rim, Z: z},
				Max: engine.Vector3{X: x + rim, Y: maximum.Y - rim, Z: z + rim},
			}, frame)
		}
	}
}

func addCratePiece(world *engine.World, cube *mesh.Mesh, name string, bounds collision.Bounds, surface material.Material) {
	minimum, maximum := bounds.Min, bounds.Max
	world.Add(cube.CreateInstance(name), engine.Transform{
		Position: engine.Vector3{
			X: (minimum.X + maximum.X) * boxMiddle,
			Y: (minimum.Y + maximum.Y) * boxMiddle,
			Z: (minimum.Z + maximum.Z) * boxMiddle,
		},
		Scale: engine.Vector3{X: maximum.X - minimum.X, Y: maximum.Y - minimum.Y, Z: maximum.Z - minimum.Z},
	}, surface)
}

// gatePanelBounds leaves a visible reveal inside the prepared collider opening.
func gatePanelBounds(opening collision.Bounds) collision.Bounds {
	opening.Min.X += gateReveal
	opening.Max.X -= gateReveal
	opening.Min.Y += gateReveal

	return opening
}

func gateFrameBounds(opening collision.Bounds) ([2]collision.Bounds, collision.Bounds) {
	centerZ := (opening.Min.Z + opening.Max.Z) * boxMiddle
	jambTop := headerBottom - gateReveal
	jambHeight := jambTop - gateReveal
	jambCenterY := (jambTop + gateReveal) * boxMiddle
	jambs := [2]collision.Bounds{
		layout.Block(opening.Min.X-gateReveal-jambWidth*boxMiddle, jambCenterY, centerZ, jambWidth, jambHeight, jambDepth),
		layout.Block(opening.Max.X+gateReveal+jambWidth*boxMiddle, jambCenterY, centerZ, jambWidth, jambHeight, jambDepth),
	}
	headerOverhang := jambWidth + gateReveal + gateReveal
	width := opening.Max.X - opening.Min.X + headerOverhang + headerOverhang
	header := layout.Block(
		(opening.Min.X+opening.Max.X)*boxMiddle, (headerBottom+headerTop)*boxMiddle, centerZ,
		width, headerTop-headerBottom, headerDepth,
	)

	return jambs, header
}

func (art courtyardArt) addGateTrim(
	world *engine.World, name string, opening collision.Bounds, color engine.Color, trim material.Material,
) {
	jambs, header := gateFrameBounds(opening)
	for _, jamb := range jambs {
		addBlock(world, name+" jamb", jamb, albedo(art.panel, color))
	}

	addBlock(world, name+" lintel", header, trim)
}

//nolint:mnd // Authored exit, threshold and decal placements within the existing course.
func (art courtyardArt) addDetails(world *engine.World) {
	trim := albedo(art.hazard, engine.ColorWhite())
	trim.UVScale = engine.Vector2{X: 3, Y: 1}
	art.addGateTrim(world, "copper", layout.FirstGate(), copper, trim)
	art.addGateTrim(world, "violet", layout.TimedGate(), violet, trim)

	for _, side := range []float32{-1, 1} {
		addBlock(world, "exit column", layout.Block(side*1.6, .75, -11.1, .22, 1.5, .22), albedo(art.panel, cyan))
	}

	for _, position := range []float32{3.62, -3.91} {
		addBlock(world, "hazard threshold", layout.Block(0, .012, position, 2.58, .018, .3), trim)
	}

	addFloorDecal(world, "approach arrow", engine.Vector3{Y: .012, Z: 5.52}, 1.65, math.Pi/2, art.arrow)
	addFloorDecal(world, "exit waypoint", engine.Vector3{Y: .057, Z: -11.05}, 1.25, 0, art.waypoint)
}

func addFloorDecal(world *engine.World, name string, position engine.Vector3, size, yaw float32, image *texture.Texture) {
	const maskCutoff = .45

	pose := engine.DefaultTransform()
	pose.Position, pose.Rotation.Y = position, yaw
	surface := albedo(image, engine.ColorWhite())
	surface.AlphaMask, surface.AlphaCutoff = true, maskCutoff
	world.Add(mesh.NewPlane(name, mesh.PlaneOptions{Width: size, Height: size}), pose, surface)
}
