package particledata

import (
	"encoding/binary"
	"math"
	"testing"
	"time"
)

func TestWorldParticleStepSharesTheScreenCommandSize(t *testing.T) {
	t.Parallel()

	effect := &Effect{Duration: 1, Layers: []Layer{{
		Capacity: 1, MaxLife: .5, Bursts: []Burst{{Count: 1}},
	}}}
	instance := NewInstance(effect, [3]float32{2, -3, 4}, .25)
	instance.Advance(time.Second/60, 1200, 800)

	data := instance.Steps()[0].Bytes
	for index, want := range [4]float32{2, -3, 4, .25} {
		if got := math.Float32frombits(binary.LittleEndian.Uint32(data[(2+index)*4:])); got != want {
			t.Fatalf("placement word %d = %v, want %v", index, got, want)
		}
	}
}

func TestCameraCachesProjectionAndNormalizesScale(t *testing.T) {
	t.Parallel()

	var camera Camera

	matrix := [16]float32{0, 0, 1, 1, 0, 3, 0, 0, -2, 0, 0, 0, 0, 0, 1, 1}
	if !camera.Update(matrix) || camera.Update(matrix) {
		t.Fatal("a stationary projection must reuse the camera bytes")
	}

	for index, want := range [8]float32{0, 0, -1, 0, 0, 1, 0, 0} {
		word := math.Float32frombits(binary.LittleEndian.Uint32(camera.Bytes[cameraRightOffset+index*wgslFloatSize:]))
		if word != want {
			t.Fatalf("camera axis word %d = %v, want %v", index, word, want)
		}
	}
}
