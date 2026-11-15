//nolint:mnd // Scan geometry, animation timing and color palettes are authored values.
package main

import (
	"math"

	"github.com/karty-game/karty-webgpu/pkg/ui"
)

func (study *expedition) animateRings(unit float64) {
	for index, ring := range study.rings {
		phase := math.Mod(study.elapsed*.18+float64(index)/3, 1)
		size := float32((70 + phase*330) * unit / 256)
		ring.SetPosition(study.sonarX*float32(study.width), study.sonarY*float32(study.height)).
			SetScale(size, size).SetOpacity(float32((1 - phase) * .4))
	}
}

func (study *expedition) layoutScan() {
	width, height := float32(study.width), float32(study.height)
	left, top, scanWidth, scanHeight := width*.48, height*.17, width*.43, height*.56

	for _, ring := range study.rings {
		if study.scanned {
			ring.SetClip(int(left), int(top), int(scanWidth), int(scanHeight))
		} else {
			ring.ClearClip()
		}
	}

	corners := [4][2]float32{{left, top}, {left + scanWidth, top}, {left, top + scanHeight}, {left + scanWidth, top + scanHeight}}
	length := min(width, height) * .025

	for index, point := range corners {
		horizontal, vertical := study.frame[index*2], study.frame[index*2+1]

		directionX, directionY := float32(1), float32(1)
		if index%2 == 1 {
			directionX = -1
		}

		if index >= 2 {
			directionY = -1
		}

		horizontal.SetPosition(point[0], point[1]).SetScale(length*directionX, 1).SetVisible(study.scanned)
		vertical.SetPosition(point[0], point[1]).SetScale(1, length*directionY).SetVisible(study.scanned)
	}
}

func (study *expedition) applyLayers() {
	study.showHabitat(!study.flat)

	for _, sprite := range study.particles {
		sprite.sprite.SetVisible(!study.flat)
	}

	study.farReef.SetVisible(!study.flat)
	study.nearReef.SetVisible(!study.flat)
	study.coralLeft.SetVisible(!study.flat)
	study.coralRight.SetVisible(!study.flat)
}

func (study *expedition) setPalette(index int) {
	palettes := [3][3]float32{{.6, 1, 1}, {.55, 1, .65}, {1, .65, .3}}
	tint := palettes[index]
	study.jelly.SetTint(tint[0], tint[1], tint[2])

	for _, ring := range study.rings {
		ring.SetTint(tint[0], tint[1], tint[2])
	}

	study.halo.SetTint(tint[0]*.3, tint[1]*.7, tint[2]*.8)
	study.dirty = true
}

// The same routed pointer event supports mouse, pen and touch, with UI taking priority.
func (study *expedition) pointSonar(event *ui.RoutedEvent) {
	pointer := event.Event
	if pointer.Kind != ui.PointerDown || pointer.Button != 0 || study.view.ConsumesPointer(pointer.X, pointer.Y) {
		return
	}

	bounds := study.view.Root().Bounds()

	width, height := study.screen.Size()
	if bounds.W <= 0 || bounds.H <= 0 || width <= 0 || height <= 0 {
		return
	}

	// Returning to the water hands keyboard shortcuts back to the scene.
	study.view.SetFocus(nil)

	x, y := study.screen.FromLogical(pointer.X, pointer.Y, bounds.W, bounds.H)
	study.sonarX = max(0, min(1, x/float32(width)))
	study.sonarY = max(0, min(1, y/float32(height)))
	study.dirty = true
}
