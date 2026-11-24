package particledata

import (
	"encoding/binary"
	"testing"
	"time"
)

func TestStoppedPrewarmedEffectRestartsWithBakedSlots(t *testing.T) {
	t.Parallel()

	effect := &Effect{Duration: .5, Layers: []Layer{{
		Capacity: 20, MaxLife: .6, WarmLife: .12, Warm: make([]byte, StateHeader+2*ParticleBytes),
	}}}
	instance := NewInstance(effect, [3]float32{}, 1)
	instance.Stop()
	instance.Advance(time.Second, 800, 600)

	if instance.Visible() || instance.Steps()[0].Reset {
		t.Fatal("stopped effect scheduled work")
	}

	instance.Restart()
	instance.Advance(16*time.Millisecond, 800, 600)

	step := instance.Steps()[0]
	if !step.Active || !step.Reset || step.Slots != 2 || step.Births != 0 || binary.LittleEndian.Uint32(step.Bytes[28:]) != 3 {
		t.Fatalf("restart did not restore the baked prefix: %+v", step)
	}

	for range 10 {
		instance.Advance(16*time.Millisecond, 800, 600)
	}

	if instance.Visible() {
		t.Fatal("prewarmed tail kept scheduling beyond its lifetime")
	}
}

func TestEmissionWrapAndRestart(t *testing.T) {
	t.Parallel()

	effect := &Effect{Duration: .1, Loop: true, Layers: []Layer{{
		Capacity: 20, Rate: 20, Delay: .025, MaxLife: .2,
		Bursts: []Burst{{Time: .025, Count: 3}},
	}}}

	instance := NewInstance(effect, [3]float32{}, 0)
	for index, want := range []uint32{3, 1, 4, 1} {
		instance.Advance(50*time.Millisecond, 800, 600)

		step := instance.Steps()[0]
		if step.Births != want || !step.Active || step.Reset != (index == 0) {
			t.Fatalf("step %d births=%d active=%v reset=%v, want births=%d", index, step.Births, step.Active, step.Reset, want)
		}
	}

	instance.Restart()
	instance.Advance(50*time.Millisecond, 800, 600)

	if step := instance.Steps()[0]; !step.Reset || step.Births != 3 || step.Slots != 3 {
		t.Fatalf("restart: births=%d slots=%d reset=%v", step.Births, step.Slots, step.Reset)
	}
}

func TestOneShotStopsSchedulingAfterItsTail(t *testing.T) {
	t.Parallel()

	instance := NewInstance(&Effect{Duration: .1, Layers: []Layer{{
		Capacity: 1, MaxLife: .05, Bursts: []Burst{{Count: 1}},
	}}}, [3]float32{}, 0)
	instance.Advance(time.Second, 10, 10)

	if step := instance.Steps()[0]; !step.Active || step.Births != 1 {
		t.Fatal("birth at time zero was lost")
	}

	for range 3 {
		instance.Advance(50*time.Millisecond, 10, 10)
	}

	if step := instance.Steps()[0]; step.Active || step.Reset || step.Births != 0 {
		t.Fatal("finished effect kept scheduling work")
	}
}
