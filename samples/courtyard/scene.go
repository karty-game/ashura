package main

import (
	"math"
	"time"

	"github.com/karty-game/ashura/pkg/camera"
	"github.com/karty-game/ashura/pkg/collision"
	"github.com/karty-game/ashura/pkg/engine"
	"github.com/karty-game/ashura/pkg/light"
	"github.com/karty-game/ashura/pkg/material"
	"github.com/karty-game/ashura/pkg/mesh"
	"github.com/karty-game/ashura/samples/courtyard/generated/assets/colliders"
	"github.com/karty-game/ashura/samples/courtyard/layout"
)

const (
	solid collision.Layers = 1 << iota
	trigger
	switchTarget
)

const (
	walkSpeed  = 4
	aimReach   = 4
	eyeOffset  = .67
	lookSpeed  = .0035
	maxPitch   = 1.5
	startPitch = -.14
	boxMiddle  = .5
	relicCount = 3
	routeCount = 2
	hiddenY    = -3
	gateWindow = 5 * time.Second
)

//nolint:gochecknoglobals,mnd // Authored colors and capsule dimensions for the courtyard.
var (
	walkingShape = collision.Capsule{Radius: .32, HalfSegment: .53}
	copper       = engine.Color{R: .52, G: .24, B: .08, A: 1}
	violet       = engine.Color{R: .34, G: .16, B: .73, A: 1}
	cyan         = engine.Color{R: .05, G: .51, B: .64, A: 1}
	cyanLit      = engine.Color{R: .20, G: .93, B: .98, A: 1}
	gold         = engine.Color{R: 1, G: .66, B: .09, A: 1}
	green        = engine.Color{R: .09, G: .77, B: .31, A: 1}
)

type gate struct {
	mesh           *collision.Mesh
	pose           *engine.Transform
	id             collision.ID
	restY, raisedY float32
}

func (door *gate) setOpen(world *collision.World, open bool) {
	if open && door.id != 0 {
		world.Remove(door.id)
		door.id = 0
		door.pose.Position.Y = door.raisedY
	} else if !open && door.id == 0 {
		door.id = world.AddMesh(door.mesh, solid)
		door.pose.Position.Y = door.restY
	}
}

type relic struct {
	pose    *engine.Transform
	overlap collision.ID
	height  float32
	taken   bool
}

type courtyard struct {
	world        *engine.World
	camera       *engine.Transform
	switchColor  *material.Material
	exitColor    *material.Material
	plateColors  [2]*material.Material
	player       *collision.Character
	queries      collision.World
	first, timed gate
	relics       [routeCount][relicCount]relic
	best         [routeCount]time.Duration
	plateIDs     [2]collision.ID
	overlaps     [routeCount*relicCount + 4]collision.ID
	switchID     collision.ID
	doorway      collision.ID
	exit         collision.ID
	gateTime     time.Duration
	elapsed      time.Duration
	route        int
	collected    int
	switchReady  bool
	started      bool
	finished     bool
}

func buildCourtyard(art courtyardArt) (*courtyard, error) {
	world := engine.NewWorld()
	scene := &courtyard{world: world}
	scene.queries.AddMesh(colliders.Stone(), solid)
	art.buildYards(world)
	scene.buildChallenge(art)
	art.addDetails(world)

	projection, _ := camera.FirstPerson()

	view := world.Add(engine.DefaultTransform(), projection)
	if err := world.SetActiveCamera(view); err != nil {
		return nil, err
	}

	scene.camera = world.Transform(view)
	scene.reset(false)

	sky := light.DefaultHemisphere("sky")
	world.Add(sky)
	world.Add(light.DefaultDirectional("sun"))

	return scene, nil
}

func (scene *courtyard) buildChallenge(art courtyardArt) {
	scene.first = scene.newGate("copper gate", layout.FirstGate(), colliders.FirstGate(), albedo(art.copperGate, engine.ColorWhite()))
	scene.timed = scene.newGate("violet gate", layout.TimedGate(), colliders.TimedGate(), albedo(art.violetGate, engine.ColorWhite()))
	scene.switchID = scene.queries.AddMesh(colliders.Switch(), switchTarget)
	_, scene.switchColor = addBlock(scene.world, "cyan switch", layout.Switch(), albedo(art.panel, cyan))
	scene.doorway = scene.queries.AddTrigger(floorTrigger(layout.TimedGate()), trigger)
	scene.exit = scene.queries.AddTrigger(floorTrigger(layout.Finish()), trigger)
	_, scene.exitColor = addBlock(scene.world, "exit pad", layout.Finish(), albedo(art.panel, cyan))

	for index, pad := range layout.Plates() {
		scene.plateIDs[index] = scene.queries.AddTrigger(floorTrigger(pad), trigger)
		_, scene.plateColors[index] = addBlock(scene.world, "pressure plate", pad, albedo(art.panel, violet))
	}

	for route, positions := range layout.Relics() {
		for index, position := range positions {
			scene.relics[route][index] = scene.newRelic(position)
		}
	}
}

func (scene *courtyard) newGate(name string, box collision.Bounds, prepared *collision.Mesh, surface material.Material) gate {
	panel := gatePanelBounds(box)
	pose, _ := addBlock(scene.world, name, panel, surface)
	raisedY := headerTop + gateReveal + (panel.Max.Y-panel.Min.Y)*boxMiddle

	return gate{mesh: prepared, pose: pose, restY: pose.Position.Y, raisedY: raisedY}
}

func (scene *courtyard) newRelic(position engine.Vector3) relic {
	const (
		span     = .76
		diameter = .52
		segments = 8
	)

	box := layout.Block(position.X, position.Y, position.Z, span, span, span)
	surface := material.DefaultStandard()
	surface.Color = gold
	transform := engine.DefaultTransform()
	transform.Position = position
	entity := scene.world.Add(mesh.NewSphere("gold relic", mesh.SphereOptions{Diameter: diameter, Segments: segments}), transform, surface)

	return relic{pose: scene.world.Transform(entity), height: position.Y, overlap: scene.queries.AddTrigger(box, trigger)}
}

func floorTrigger(box collision.Bounds) collision.Bounds {
	const triggerHeight = 1.2

	box.Min.Y, box.Max.Y = 0, triggerHeight

	return box
}

func addBlock(
	world *engine.World,
	name string,
	bounds collision.Bounds,
	surface material.Material,
) (*engine.Transform, *material.Material) {
	lo, hi := bounds.Min, bounds.Max
	transform := engine.DefaultTransform()
	transform.Position = engine.Vector3{X: (lo.X + hi.X) * boxMiddle, Y: (lo.Y + hi.Y) * boxMiddle, Z: (lo.Z + hi.Z) * boxMiddle}
	box := mesh.NewBox(name, mesh.BoxOptions{Width: hi.X - lo.X, Height: hi.Y - lo.Y, Depth: hi.Z - lo.Z})
	entity := world.Add(box, transform, surface)
	stored, _ := engine.Get[material.Material](world, entity)

	return world.Transform(entity), stored
}

func (scene *courtyard) rayDirection() engine.Vector3 {
	yaw, pitch := float64(scene.camera.Rotation.Y), float64(scene.camera.Rotation.X)
	cosPitch := math.Cos(pitch)

	return engine.Vector3{
		X: float32(-math.Sin(yaw) * cosPitch * aimReach),
		Y: float32(math.Sin(pitch) * aimReach),
		Z: float32(-math.Cos(yaw) * cosPitch * aimReach),
	}
}
