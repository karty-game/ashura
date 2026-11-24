package main

import (
	"time"

	"github.com/karty-game/ashura/pkg/camera"
	"github.com/karty-game/ashura/pkg/collision"
	"github.com/karty-game/ashura/pkg/decal"
	"github.com/karty-game/ashura/pkg/engine"
	"github.com/karty-game/ashura/pkg/texture"
	"github.com/karty-game/ashura/pkg/ui"
	authored "github.com/karty-game/ashura/samples/decals/generated/assets/decals"
	"github.com/karty-game/ashura/samples/decals/internal/room"
)

const yardCollisionLayer collision.Layers = 1

const (
	sprayColors         = 3
	strokeCellsPerFrame = 28
	clearCellsPerFrame  = 128
)

type paintPanel struct {
	layout     room.Panel
	placements [sprayColors]*decal.Placement
	pigment    []uint8
}

type sprayer struct {
	panels                                         [3]paintPanel
	details                                        *decal.Placement
	detailBatch                                    *decal.Prepared
	atlas                                          *texture.Texture
	world                                          *engine.World
	lastPanel, lastCell                            int
	color                                          uint8
	painted                                        int
	wantDetails, clearPending, clearHeld, clearing bool
	clearPanel, clearCell                          int
	status                                         *ui.Node
	shown                                          paintStatus
}

func attachSprayer(
	app *engine.App, world *engine.World, view engine.Entity, queries *collision.World, atlas *texture.Texture,
) error {
	details, err := authored.YardDetails()
	if err != nil {
		return err
	}

	tool := &sprayer{detailBatch: details, atlas: atlas, world: world, lastPanel: -1, color: 1, wantDetails: true}
	if err := tool.updateDetails(); err != nil {
		return err
	}

	if err := tool.mountPaint(); err != nil {
		return err
	}

	if err := tool.controls(app); err != nil {
		return err
	}

	projection, _ := engine.Get[camera.Perspective](world, view)

	app.OnUpdateError(func(time.Duration) error {
		return tool.update(app.Input(), world.Transform(view), queries, projection.FieldOfViewDegrees)
	})

	return nil
}

func (tool *sprayer) mountPaint() error {
	sources := [3][sprayColors]func() (*decal.Prepared, error){
		{authored.MuralCyan, authored.MuralPink, authored.MuralGold},
		{authored.ServiceCyan, authored.ServicePink, authored.ServiceGold},
		{authored.FreightCyan, authored.FreightPink, authored.FreightGold},
	}

	for index, layout := range room.Panels() {
		panel := &tool.panels[index]
		panel.layout = layout
		panel.pigment = make([]uint8, layout.Count())

		for colorIndex, load := range sources[index] {
			prepared, err := load()
			if err != nil {
				return err
			}

			panel.placements[colorIndex], err = tool.world.MountDisabledDecals(prepared, tool.atlas)
			if err != nil {
				return err
			}
		}
	}

	return nil
}

func (tool *sprayer) updateDetails() (err error) {
	if tool.wantDetails && tool.details == nil {
		tool.details, err = tool.world.MountDecals(tool.detailBatch, tool.atlas)
	} else if !tool.wantDetails && tool.details != nil {
		tool.details.Remove()
		tool.details = nil
	}

	return err
}

func (tool *sprayer) update(input engine.Input, pose *engine.Transform, queries *collision.World, fov float32) error {
	if err := tool.updateDetails(); err != nil {
		return err
	}

	tool.readKeys(input)

	if tool.clearPending {
		tool.clearPending, tool.clearing, tool.lastPanel = false, true, -1
		tool.clearPanel, tool.clearCell = 0, 0
	}

	var err error
	if tool.clearing {
		err = tool.clearStep()
	} else {
		err = tool.spray(input, pose, queries, fov)
	}

	if err != nil {
		return err
	}

	return tool.updateStatus()
}

func (tool *sprayer) readKeys(input engine.Input) {
	switch {
	case input.KeyDown(engine.Key1):
		tool.color = 1
	case input.KeyDown(engine.Key2):
		tool.color = 2
	case input.KeyDown(engine.Key3):
		tool.color = 3
	}

	held := input.KeyDown(engine.KeyR)
	tool.clearPending = tool.clearPending || held && !tool.clearHeld
	tool.clearHeld = held
}

func (tool *sprayer) clearStep() error {
	for range clearCellsPerFrame {
		if tool.clearPanel == len(tool.panels) {
			tool.clearing = false

			return nil
		}

		panel := &tool.panels[tool.clearPanel]
		if err := tool.setPigment(panel, tool.clearCell, 0); err != nil {
			return err
		}

		tool.clearCell++
		if tool.clearCell == len(panel.pigment) {
			tool.clearPanel++
			tool.clearCell = 0
		}
	}

	return nil
}

func (tool *sprayer) spray(input engine.Input, pose *engine.Transform, queries *collision.World, fov float32) error {
	index, cell := tool.pick(input, pose, queries, fov)
	if index < 0 {
		tool.lastPanel = -1

		return nil
	}

	panel := &tool.panels[index]
	if tool.lastPanel != index {
		tool.lastPanel, tool.lastCell = index, cell
	}

	var err error

	tool.lastCell, err = tool.stroke(panel, tool.lastCell, cell)

	return err
}

func (tool *sprayer) pick(
	input engine.Input, pose *engine.Transform, queries *collision.World, fov float32,
) (panel, cell int) {
	width, height := input.LogicalViewportSize()

	pointer, touching := input.TouchPosition()
	if !touching {
		if !input.MouseButtonDown(engine.MouseButtonLeft) || input.MouseButtonDown(engine.MouseButtonRight) {
			return -1, 0
		}

		pointer = input.MousePosition()
	}

	if width <= 0 || height <= 0 || pointer.X < 0 || pointer.Y < 0 || pointer.X >= width || pointer.Y >= height {
		return -1, 0
	}

	const reach = 54

	direction := pointerRay(pointer.X/width, pointer.Y/height, width/height, fov, pose.Rotation)
	displacement := engine.Vector3{X: direction.X * reach, Y: direction.Y * reach, Z: direction.Z * reach}

	hit, found := queries.Raycast(pose.Position, displacement, yardCollisionLayer)
	if !found {
		return -1, 0
	}

	for index := range tool.panels {
		if cell, ok := tool.panels[index].layout.Cell(hit.Point, hit.Normal); ok {
			return index, cell
		}
	}

	return -1, 0
}

func (tool *sprayer) stroke(panel *paintPanel, start, target int) (int, error) {
	columns := panel.layout.Cols
	fromX, fromY := start%columns, start/columns
	toX, toY := target%columns, target/columns
	deltaX, deltaY := toX-fromX, toY-fromY
	steps := max(max(deltaX, -deltaX), max(deltaY, -deltaY), 1)

	last := start

	for index := 0; index <= min(steps, strokeCellsPerFrame); index++ {
		x := fromX + deltaX*index/steps
		y := fromY + deltaY*index/steps
		last = y*columns + x

		if err := tool.setPigment(panel, last, tool.color); err != nil {
			return last, err
		}
	}

	return last, nil
}

func (tool *sprayer) setPigment(panel *paintPanel, index int, color uint8) error {
	before := panel.pigment[index]
	if before == color {
		return nil
	}

	if before != 0 {
		if err := panel.placements[before-1].SetEnabled(index, false); err != nil {
			return err
		}

		panel.pigment[index] = 0
		tool.painted--
	}

	if color != 0 {
		if err := panel.placements[color-1].SetEnabled(index, true); err != nil {
			return err
		}

		panel.pigment[index] = color
		tool.painted++
	}

	return nil
}
