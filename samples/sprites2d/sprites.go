//nolint:mnd // Authored composition, animation rates and palette values are intentional.
package main

import (
	"math"
	"time"

	"github.com/karty-game/ashura/pkg/draw2d"
	"github.com/karty-game/ashura/pkg/engine"
	"github.com/karty-game/ashura/pkg/ui"
	"github.com/karty-game/ashura/samples/sprites2d/generated/assets"
)

const (
	planktonCount     = 384
	fishCount         = 96
	kelpCount         = 640
	bubbleCount       = 320
	seabedDetailCount = 160
)

type swimmer struct {
	sprite            *draw2d.Sprite
	x, y, speed, size float64
	reverse           bool
}
type plankton struct {
	sprite            *draw2d.Sprite
	x, y, phase, size float64
}
type expedition struct {
	app                                                                          *engine.App
	screen                                                                       *draw2d.Screen
	images                                                                       assets.Catalog
	background, farReef, nearReef, coralLeft, coralRight, jelly, halo, sub, lamp *draw2d.Sprite
	fish                                                                         []swimmer
	particles                                                                    []plankton
	kelp                                                                         []kelpStem
	bubbles                                                                      []airBubble
	seabed                                                                       []seabedDetail
	rings                                                                        [3]*draw2d.Sprite
	frame                                                                        [8]*draw2d.Sprite
	view                                                                         *ui.View
	title, footer, readout, motion                                               *ui.Node
	elapsed, statusTime                                                          float64
	sonarX, sonarY                                                               float32
	width, height                                                                int
	logicalWidth, logicalHeight                                                  float32
	frameIndex                                                                   int
	paused, flat, scanned, spaceHeld, dirty                                      bool
}

func newExpedition(app *engine.App) *expedition {
	return &expedition{app: app, screen: app.Screen2D(), sonarX: .67, sonarY: .43, dirty: true, frameIndex: -1}
}

func (study *expedition) createSprites(images assets.Catalog) error {
	study.images = images
	layer := study.screen
	study.background = layer.NewSprite(images.Background)
	study.farReef = layer.NewSprite(images.SpritesReef).SetOrder(10).SetTint(.12, .45, .5).SetOpacity(.45)
	study.halo = layer.NewSprite(images.SpritesGlow).SetOrder(15).SetTint(.1, .7, .9).SetOpacity(.3)
	study.addParticles()
	study.addFish()
	study.addHabitat()
	study.jelly = layer.NewSprite(images.SpritesJelly0).SetOrder(40).SetPivot(200, 240)
	study.lamp = layer.NewSprite(images.SpritesGlow).SetOrder(45).SetTint(1, .66, .2).SetOpacity(.35)

	study.sub = layer.NewSprite(images.SpritesSubmarine).SetOrder(50).SetPivot(100, 50)
	for index := range study.rings {
		study.rings[index] = layer.NewSprite(images.SpritesRing).SetOrder(55).SetPivot(128, 128)
	}

	study.nearReef = layer.NewSprite(images.SpritesReef).SetOrder(60).SetTint(.05, .18, .22)
	study.coralLeft = layer.NewSprite(images.SpritesCoral).SetOrder(65).SetPivot(120, 360).SetTint(.15, .48, .48)

	study.coralRight = layer.NewSprite(images.SpritesCoral).SetOrder(65).SetPivot(120, 360).SetTint(.07, .27, .3)
	for index := range study.frame {
		study.frame[index] = layer.NewSprite(images.SpritesWhite).SetOrder(70).SetTint(.4, .9, .8).SetOpacity(.65).SetVisible(false)
	}

	study.setPalette(0)

	return study.updateReadout()
}

func (study *expedition) addParticles() {
	for index := range planktonCount {
		seed := float64(index)
		sprite := study.screen.NewSprite(study.images.SpritesGlow).SetOrder(20).SetPivot(32, 32).SetTint(.38, .83, .8)
		study.particles = append(
			study.particles,
			plankton{sprite: sprite, x: fract(seed * .618), y: fract(seed*.754877 + .07), phase: seed * 1.71, size: 1 + fract(seed*.17)*3},
		)
	}
}

func (study *expedition) addFish() {
	for index := range fishCount {
		seed := float64(index)
		sprite := study.screen.NewSprite(study.images.SpritesFish0).SetOrder(30).SetPivot(40, 16)
		study.fish = append(
			study.fish,
			swimmer{
				sprite:  sprite,
				x:       fract(seed * .618),
				y:       .24 + fract(seed*.317)*.48,
				speed:   .012 + fract(seed*.23)*.02,
				size:    .24 + fract(seed*.19)*.45,
				reverse: index%3 == 0,
			},
		)
	}
}

func fract(value float64) float64 { return value - math.Floor(value) }

func (study *expedition) update(delta time.Duration) error {
	width, height := study.screen.Size()
	if width == 0 || height == 0 {
		return nil
	}

	study.readInput()

	resized := width != study.width || height != study.height
	if resized {
		study.width, study.height = width, height
		study.layout()
		study.dirty = true
	}

	bounds := study.view.Root().Bounds()
	if bounds.W != study.logicalWidth || bounds.H != study.logicalHeight {
		study.logicalWidth, study.logicalHeight = bounds.W, bounds.H
		study.responsive()
	}

	if !study.paused {
		study.elapsed += delta.Seconds()
	}

	if !study.paused || study.dirty {
		study.animate()
		study.dirty = false
	}

	study.statusTime += delta.Seconds()
	if study.statusTime >= .5 {
		study.statusTime = 0

		return study.updateReadout()
	}

	return nil
}

func (study *expedition) readInput() {
	input := study.app.Input()

	down := input.KeyDown(engine.KeySpace)
	if down && !study.spaceHeld {
		study.setPaused(!study.paused)
	}

	study.spaceHeld = down
}

func (study *expedition) layout() {
	width, height := float32(study.width), float32(study.height)
	unit := min(width/1280, height/800)
	study.background.SetPosition(0, 0).SetScale(width/1280, height/800)
	study.farReef.SetPosition(-width*.04, height*.73).SetScale(width/1024*1.12, height/800*.95)
	study.nearReef.SetPosition(0, height*.84).SetScale(width/1024, height/800*.62)
	study.coralLeft.SetPosition(width*.06, height*1.08).SetScale(-unit*.85, unit*.85)
	study.coralRight.SetPosition(width*.96, height*1.12).SetScale(unit*1.65, unit*1.65)
	study.halo.SetPosition(width*.67, height*.4).SetPivot(32, 32).SetScale(unit*9, unit*9)
	study.layoutHabitat()
	study.applyLayers()
	study.layoutScan()
}

func (study *expedition) animate() {
	width, height := float64(study.width), float64(study.height)
	unit := min(width/1280, height/800)
	elapsed := study.elapsed

	heroUnit, subUnit, heroX := unit, unit, width*.66
	if width < height*.85 {
		heroUnit *= 1.75
		subUnit *= 1.5
		heroX = width * .58
	}

	study.jelly.SetPosition(float32(heroX+math.Sin(elapsed*.23)*16*unit), float32(height*.46+math.Sin(elapsed*.65)*12*unit)).
		SetScale(float32(heroUnit*1.12), float32(heroUnit*1.12)).SetRotation(float32(math.Sin(elapsed*.31) * .035))
	subX, subY := width*.34+math.Sin(elapsed*.24)*35*unit, height*.66+math.Sin(elapsed*.7)*7*unit
	study.sub.SetPosition(float32(subX), float32(subY)).
		SetScale(float32(subUnit*.85), float32(subUnit*.85)).
		SetRotation(float32(math.Sin(elapsed*.7) * .025))
	study.lamp.SetPivot(32, 32).
		SetPosition(float32(subX+84*subUnit), float32(subY+4*subUnit)).
		SetScale(float32(subUnit*2.8), float32(subUnit*2.8))
	study.animateHabitat(unit)
	study.animateParticles(unit)
	study.animateFish(unit)
	study.animateRings(unit)
	study.swapFrames()
}

func (study *expedition) animateParticles(unit float64) {
	if study.flat {
		return
	}

	for _, particle := range study.particles {
		x := fract(particle.x+study.elapsed*.003) * float64(study.width)
		y := fract(particle.y-study.elapsed*.006) * float64(study.height)
		scale := float32(particle.size * unit / 64)
		particle.sprite.SetPosition(float32(x), float32(y)).
			SetScale(scale, scale).
			SetOpacity(float32(.18 + .25*(1+math.Sin(study.elapsed*.8+particle.phase))*.5))
	}
}

func (study *expedition) animateFish(unit float64) {
	for _, fish := range study.fish {
		direction := 1.0
		if fish.reverse {
			direction = -1
		}

		x := fract(fish.x+study.elapsed*fish.speed*direction)*1.2 - .1
		y := fish.y + math.Sin(study.elapsed*1.8+fish.x*20)*.008
		scale := float32(fish.size * unit)
		fish.sprite.SetPosition(float32(x*float64(study.width)), float32(y*float64(study.height))).
			SetScale(scale*float32(direction), scale).
			SetRotation(float32(math.Sin(study.elapsed*1.8+fish.x*20) * .035)).
			SetOpacity(float32(.35 + fish.size*.6))
	}
}

func (study *expedition) swapFrames() {
	frame := int(study.elapsed*2) % 2
	if frame == study.frameIndex {
		return
	}

	study.frameIndex = frame

	jelly, fish := study.images.SpritesJelly0, study.images.SpritesFish0
	if frame == 1 {
		jelly, fish = study.images.SpritesJelly1, study.images.SpritesFish1
	}

	study.jelly.SetImage(jelly)

	for _, swimmer := range study.fish {
		swimmer.sprite.SetImage(fish)
	}
}
