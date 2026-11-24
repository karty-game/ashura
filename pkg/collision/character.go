package collision

import (
	"time"

	"github.com/karty-game/ashura/pkg/engine"
)

const (
	// CharacterStep is the controller's fixed simulation interval.
	CharacterStep = time.Second / 60
	maxSteps      = 8
	slideContacts = 4
	gravity       = 9.81
	minGroundY    = 0.7071067811865476 // cos(45 degrees)
)

// Character is a fixed-step walking controller. Spawn with its capsule clear
// of solid surfaces. Gravity acts along -Y and movement slides along surfaces.
type Character struct {
	position, previous vector
	capsule            Capsule
	remainder          time.Duration
	fallSpeed          float64
	grounded           bool
}

// NewCharacter returns nil for an invalid shape or nonfinite center.
func NewCharacter(shape Capsule, center engine.Vector3) *Character {
	position := fromEngine(center)
	if !shape.valid() || !position.finite() {
		return nil
	}

	return &Character{position: position, previous: position, capsule: shape}
}

// Position returns the latest simulation center for gameplay queries.
func (character *Character) Position() engine.Vector3 { return character.position.engine() }

// ViewPosition interpolates between the last two steps for presentation. It is
// up to one step behind Position and should be used for the camera or model.
func (character *Character) ViewPosition() engine.Vector3 {
	fraction := float64(character.remainder) / float64(CharacterStep)

	return character.previous.add(character.position.sub(character.previous).mul(fraction)).engine()
}

// Grounded reports whether the last step met a walkable surface.
func (character *Character) Grounded() bool { return character.grounded }

// Advance applies the requested horizontal velocity in world units per second.
// At most eight steps run per call. Time beyond that cap is discarded.
func (character *Character) Advance(world *World, elapsed time.Duration, velocity engine.Vector3, solids Layers) {
	if elapsed <= 0 {
		return
	}

	planar := fromEngine(velocity)

	planar[1] = 0
	if !planar.finite() {
		return
	}

	character.remainder += min(elapsed, maxSteps*CharacterStep)
	for character.remainder >= CharacterStep {
		character.previous = character.position
		character.step(world, planar, solids)
		character.remainder -= CharacterStep
	}
}

func (character *Character) step(world *World, velocity vector, solids Layers) {
	dt := CharacterStep.Seconds()
	character.grounded = false
	character.fallSpeed -= gravity * dt
	velocity[1] = character.fallSpeed
	remaining := velocity.mul(dt)

	var planes [slideContacts]vector
	for index := range planes {
		id, hit := world.cast(capsuleSweep(character.position, remaining, character.capsule), solids)
		if id == 0 {
			character.position = character.position.add(remaining)

			return
		}

		fraction := hit.fraction
		character.position = character.position.add(remaining.mul(fraction))

		normal := hit.normal
		if normal[1] >= minGroundY {
			character.grounded = true
			character.fallSpeed = 0
		}

		planes[index] = normal
		remaining = slide(remaining.mul(1-fraction), planes[:index+1])
	}
}

func slide(remaining vector, planes []vector) vector {
	for _, normal := range planes {
		if approach := remaining.dot(normal); approach < 0 {
			remaining = remaining.sub(normal.mul(approach))
		}
	}

	// A later projection can send the capsule into an earlier plane in a corner.
	for _, normal := range planes {
		if remaining.dot(normal) < -contactTolerance {
			return vector{}
		}
	}

	return remaining
}
