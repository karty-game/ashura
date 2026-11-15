package platform

import "math"

const touchPair = 2

// TouchPoint identifies one contact in logical viewport pixels.
type TouchPoint struct {
	ID   int64
	X, Y float32
}

// TouchTracker translates contact snapshots into frame gesture deltas. It retains
// only the previous gesture baseline and allocates no memory during Update.
type TouchTracker struct {
	count                  int
	ids                    [touchPair]int64
	centerX, centerY, span float32
}

// Reset discards contact baselines on blur/cancel. The next contact starts a new gesture.
func (tracker *TouchTracker) Reset() { *tracker = TouchTracker{} }

// Update accumulates one-finger drag, two-finger pan and logarithmic pinch.
// Changing contact count or identity rebases without producing a jump. More than
// two contacts suspend gestures until a supported snapshot establishes a baseline.
func (tracker *TouchTracker) Update(input *InputState, points []TouchPoint) {
	count := len(points)
	if count < 1 || count > touchPair {
		tracker.Reset()

		return
	}

	first := points[0]
	centerX, centerY, span := first.X, first.Y, float32(0)

	ids := [touchPair]int64{first.ID, 0}
	if count == touchPair {
		second := points[1]
		centerX = (first.X + second.X) / touchPair
		centerY = (first.Y + second.Y) / touchPair
		span = float32(math.Hypot(float64(first.X-second.X), float64(first.Y-second.Y)))
		ids[1] = second.ID
	}

	sameContacts := false
	if count == tracker.count {
		sameContacts = ids == tracker.ids || count == touchPair && ids == ([touchPair]int64{tracker.ids[1], tracker.ids[0]})
	}

	if sameContacts {
		deltaX, deltaY := centerX-tracker.centerX, centerY-tracker.centerY
		if count == 1 {
			input.TouchDX += deltaX
			input.TouchDY += deltaY
		} else {
			input.TouchPanX += deltaX

			input.TouchPanY += deltaY
			if span > 0 && tracker.span > 0 {
				input.Pinch += float32(math.Log(float64(tracker.span) / float64(span)))
			}
		}
	}

	tracker.count, tracker.ids = count, ids
	tracker.centerX, tracker.centerY, tracker.span = centerX, centerY, span
}
