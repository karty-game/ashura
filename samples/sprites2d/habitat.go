//nolint:mnd // Authored depth bands, densities, current rates and color values.
package main

import (
	"math"

	"github.com/karty-game/ashura/pkg/draw2d"
)

type kelpStem struct {
	sprite                        *draw2d.Sprite
	x, base, height, width, phase float64
}

type airBubble struct {
	sprite                   *draw2d.Sprite
	x, y, speed, size, phase float64
}

type seabedDetail struct {
	sprite     *draw2d.Sprite
	x, y, size float64
}

func (study *expedition) addHabitat() {
	study.addKelp()
	study.addBubbles()
	study.addSeabed()
}

func (study *expedition) addKelp() {
	study.kelp = make([]kelpStem, 0, kelpCount)
	orders := [4]int32{11, 25, 58, 63}

	for index := range kelpCount {
		seed := float64(index)
		band := index / (kelpCount / 4)
		depth := float64(band) / 3
		sprite := study.screen.NewSprite(study.images.SpritesKelp).SetPivot(24, 160).SetOrder(orders[band]).
			SetTint(float32(.13+depth*.08), float32(.36+depth*.15), float32(.36+depth*.06)).SetOpacity(float32(.35 + depth*.45))
		study.kelp = append(study.kelp, kelpStem{
			sprite: sprite,
			x:      fract(seed * .6180339),
			base:   .82 + float64(band)*.055 + fract(seed*.173)*.025,
			height: (45 + depth*120) * (.55 + fract(seed*.317)*.65) * (.5 + .5*math.Abs(math.Sin(fract(seed*.6180339)*9+depth))),
			width:  .25 + fract(seed*.413)*.4,
			phase:  seed * 2.39,
		})
	}
}

func (study *expedition) addBubbles() {
	study.bubbles = make([]airBubble, 0, bubbleCount)
	for index := range bubbleCount {
		seed := float64(index)
		size := 4 + math.Pow(fract(seed*.173+.13), 2)*14

		order := int32(22)
		if index >= bubbleCount*3/4 {
			order = 52
		}

		sprite := study.screen.NewSprite(study.images.SpritesBubble).SetPivot(16, 16).SetOrder(order).
			SetTint(.5, .85, .86).SetOpacity(float32(.12 + size*.012))
		study.bubbles = append(study.bubbles, airBubble{
			sprite: sprite,
			x:      fract(seed*.6180339 + .11),
			y:      fract(seed*.41421356 + .03),
			speed:  .012 + size*.0013,
			size:   size,
			phase:  seed * 1.73,
		})
	}
}

func (study *expedition) addSeabed() {
	study.seabed = make([]seabedDetail, 0, seabedDetailCount)
	for index := range seabedDetailCount {
		seed := float64(index)
		sprite := study.screen.NewSprite(study.images.SpritesPebble).SetPivot(20, 12).SetOrder(61).
			SetTint(float32(.16+fract(seed*.17)*.12), float32(.32+fract(seed*.21)*.15), .35).
			SetOpacity(.65).SetRotation(float32(math.Sin(seed*1.71) * .45))
		study.seabed = append(study.seabed, seabedDetail{
			sprite: sprite, x: fract(seed * .6180339), y: .89 + fract(seed*.317)*.1, size: .25 + fract(seed*.271)*.8,
		})
	}
}

func (study *expedition) layoutHabitat() {
	width, height := float64(study.width), float64(study.height)

	unit := min(width/1280, height/800)
	for _, stem := range study.kelp {
		stem.sprite.SetPosition(float32(stem.x*width), float32(stem.base*height)).
			SetScale(float32(stem.width*unit), float32(stem.height*unit/160))
	}

	for _, detail := range study.seabed {
		scale := float32(detail.size * unit)
		detail.sprite.SetPosition(float32(detail.x*width), float32(detail.y*height)).SetScale(scale, scale)
	}
}

func (study *expedition) animateHabitat(unit float64) {
	if study.flat {
		return
	}

	for _, stem := range study.kelp {
		sway := math.Sin(study.elapsed*.22+stem.phase)*.065 + math.Sin(study.elapsed*.41+stem.phase*1.3)*.012
		stem.sprite.SetRotation(float32(sway))
	}

	for _, bubble := range study.bubbles {
		x := bubble.x*float64(study.width) + math.Sin(study.elapsed*.65+bubble.phase)*8*unit
		y := fract(bubble.y-study.elapsed*bubble.speed) * float64(study.height)
		scale := float32(bubble.size * unit / 32)
		bubble.sprite.SetPosition(float32(x), float32(y)).SetScale(scale, scale)
	}
}

func (study *expedition) showHabitat(visible bool) {
	for _, stem := range study.kelp {
		stem.sprite.SetVisible(visible)
	}

	for _, bubble := range study.bubbles {
		bubble.sprite.SetVisible(visible)
	}

	for _, detail := range study.seabed {
		detail.sprite.SetVisible(visible)
	}
}
