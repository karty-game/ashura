package draw2d

import (
	"cmp"
	"encoding/binary"
	"math"
	"slices"

	"github.com/karty-game/ashura/internal/spritepaint"
)

// Frame borrows retained storage until the next Frame or mutation. Idle frames do
// not scan sprites, pack records or allocate. Resizes recompute screen clips.
func (screen *Screen) Frame(width, height int) *spritepaint.Frame {
	screen.Resize(width, height)

	if len(screen.dirty) == 0 && !screen.orderDirty && !screen.batchDirty {
		return &screen.frame
	}

	screen.frame.Base = screen.frame.Revision
	screen.frame.Revision++
	screen.frame.Width, screen.frame.Height = float32(width), float32(height)
	screen.frame.Changes = screen.frame.Changes[:0]
	screen.packDirty()

	if screen.orderDirty {
		screen.refreshOrder()
	}

	if screen.pagesDirty {
		clear(screen.frame.Textures)

		screen.frame.Textures = screen.frame.Textures[:0]
		for page := range screen.pages {
			screen.frame.Textures = append(screen.frame.Textures, page)
		}

		screen.pagesDirty = false
		screen.batchDirty = true
	}

	if screen.batchDirty {
		screen.buildBatches()
		screen.batchDirty = false
	}

	return &screen.frame
}

func (screen *Screen) packDirty() {
	slices.Sort(screen.dirty)

	previousSlot := -1
	for _, slot := range screen.dirty {
		if slot == previousSlot {
			continue
		}

		previousSlot = slot
		sprite := screen.slots[slot]

		data := screen.frame.Data[slot*spritepaint.Stride : (slot+1)*spritepaint.Stride]
		if sprite == nil {
			clear(data)
		} else {
			sprite.dirty = false
			active := sprite.visible && sprite.opacity > 0 && sprite.scaleX != 0 && sprite.scaleY != 0
			clip := screen.clipFor(sprite)

			active = active && clip.w > 0 && clip.h > 0
			if active != sprite.active {
				screen.batchDirty = true
				sprite.active = active
			}

			packSprite(data, sprite, clip)
		}

		screen.stats.PackedRecords++
		screen.addChange(slot*spritepaint.Stride, spritepaint.Stride)
	}

	screen.dirty = screen.dirty[:0]
}

func (screen *Screen) refreshOrder() {
	clear(screen.ordered)

	screen.ordered = screen.ordered[:0]
	for _, sprite := range screen.slots {
		if sprite != nil {
			screen.ordered = append(screen.ordered, sprite)
		}
	}

	slices.SortFunc(screen.ordered, func(left, right *Sprite) int {
		if order := cmp.Compare(left.order, right.order); order != 0 {
			return order
		}

		return cmp.Compare(left.sequence, right.sequence)
	})
	screen.orderDirty = false
}

func (screen *Screen) addChange(offset, size int) {
	changes := screen.frame.Changes
	if len(changes) > 0 && changes[len(changes)-1].Offset+changes[len(changes)-1].Size == offset {
		changes[len(changes)-1].Size += size
	} else {
		screen.frame.Changes = append(screen.frame.Changes, spritepaint.Range{Offset: offset, Size: size})
	}
}

//nolint:gosec // clipFor confines nonnegative scissors to the validated framebuffer (maximum side 16384).
func (screen *Screen) buildBatches() {
	screen.frame.BatchesRevision++
	screen.stats.BatchBuilds++

	clear(screen.frame.Batches)

	screen.frame.Batches = screen.frame.Batches[:0]
	for _, sprite := range screen.ordered {
		if !sprite.active {
			continue
		}

		clip := screen.clipFor(sprite)
		scissor := [4]uint32{uint32(clip.x), uint32(clip.y), uint32(clip.w), uint32(clip.h)}

		batches := screen.frame.Batches
		if len(batches) > 0 {
			previous := &batches[len(batches)-1]
			if previous.First+previous.Count == sprite.slot && previous.Texture == sprite.image.page && previous.Clip == scissor {
				previous.Count++

				continue
			}
		}

		screen.frame.Batches = append(
			screen.frame.Batches,
			spritepaint.Batch{First: sprite.slot, Count: 1, Texture: sprite.image.page, Clip: scissor},
		)
	}
}

func packSprite(data []byte, sprite *Sprite, clip rectangle) {
	sine, cosine := math.Sincos(float64(sprite.rotation))
	m00, m01 := float32(cosine)*sprite.scaleX, float32(sine)*sprite.scaleX
	m10, m11 := -float32(sine)*sprite.scaleY, float32(cosine)*sprite.scaleY
	image := sprite.image

	values := [20]float32{
		m00,
		m01,
		m10,
		m11,
		sprite.x - m00*sprite.pivotX - m10*sprite.pivotY,
		sprite.y - m01*sprite.pivotX - m11*sprite.pivotY,
		float32(image.width),
		float32(image.height),
		float32(
			image.x,
		) / float32(
			image.page.Width,
		),
		float32(image.y) / float32(image.page.Height),
		float32(image.width) / float32(image.page.Width),
		float32(image.height) / float32(image.page.Height),
		sprite.tint[0],
		sprite.tint[1],
		sprite.tint[2],
		sprite.opacity,
		float32(clip.x),
		float32(clip.y),
		float32(clip.w),
		float32(clip.h),
	}
	for index, value := range values {
		binary.LittleEndian.PutUint32(data[index*4:], math.Float32bits(value))
	}
}
