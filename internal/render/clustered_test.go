package render

import (
	"encoding/binary"
	"math"
	"testing"

	"github.com/karty-game/ashura/internal/meshdata"
)

func TestClusteredMasksInvalidateOnlyForInfluenceChanges(t *testing.T) {
	t.Parallel()

	state := &ClusterFrameState{}
	frame := Frame{
		ViewProjection: [16]float32{0: 1, 5: 1, 10: 1, 15: 1},
		CameraNear:     .1, CameraFar: 90,
		Lights: make([]Light, ClusterLightLimit),
	}
	frame.Lights[0] = Light{Kind: PointLightKind, Intensity: 1, GroundColor: [3]float32{2, 0, 0}}

	var spots [SpotShadowCount]SpotView

	check := func(upload, dispatch bool) {
		t.Helper()

		if write, build := state.Update(frame, 1280, 720, spots); write != upload || build != dispatch {
			t.Fatalf("upload, dispatch = %v, %v, want %v, %v", write, build, upload, dispatch)
		}
	}
	check(true, true)

	if state.Groups != ([3]uint32{80, 45, 1}) {
		t.Fatalf("unexpected workgroups %v", state.Groups)
	}

	check(false, false)

	frame.Lights[0].Intensity *= 2

	check(true, false)

	frame.Lights[0].Direction[0]++

	check(true, true)

	frame.UnculledLights = true

	check(true, false)

	frame.Lights[ClusterLightLimit-1] = frame.Lights[0]
	frame.UnculledLights = false

	check(true, true)

	if got := math.Float32frombits(binary.LittleEndian.Uint32(state.Data[23*4:])); got != ClusterLightLimit {
		t.Fatalf("high-word light was lost, light count %v", got)
	}

	frame.Lights[0].Intensity, frame.Lights[ClusterLightLimit-1].Intensity = 0, 0

	check(true, false)

	frame.Lights[0].Intensity = 1

	check(true, true)
}

func TestSpotShadowCacheTracksRelevantCasterChanges(t *testing.T) {
	t.Parallel()

	spot := Light{
		Kind: SpotLightKind, Intensity: 1, CastShadows: true, GroundColor: [3]float32{10, 0, 0},
		SpotDirection: [3]float32{0, 0, -1}, InnerAngle: .25, OuterAngle: .45,
	}
	second := spot
	second.Direction[0] = 20
	draw := DrawCall{
		ID: 1, Vertices: []Vertex{{}}, Indices: []uint32{0, 0, 0},
		Bounds:    meshdata.Bounds{Minimum: [3]float32{-.25, -.25, -.25}, Maximum: [3]float32{.25, .25, .25}, Valid: true},
		Transform: [16]float32{0: 1, 5: 1, 10: 1, 14: -2, 15: 1},
	}
	frame := Frame{Lights: []Light{spot, second}, Draws: []DrawCall{draw}, ShadowCasterRevision: 1}

	var state SpotShadowState

	check := func(wantDirty, wantMask uint8) {
		t.Helper()

		dirty, masks := state.Update(frame)
		if dirty != wantDirty || dirty != 0 && (len(masks) != 1 || masks[0] != wantMask) {
			t.Fatalf("dirty %v masks %v, want %v / %v", dirty, masks, wantDirty, wantMask)
		}
	}
	check(3, 1)

	frame.CameraPosition[0]++
	frame.Lights[0].Color[1] = .5

	check(0, 0)

	frame.Draws[0].Transform[12] = 20
	frame.ShadowCasterRevision++

	check(3, 2)

	frame.Draws[0].Transform[12] = 21
	frame.ShadowCasterRevision++

	check(2, 2)

	frame.Lights[1].Intensity = 0

	check(0, 0)

	frame.Lights[1].Intensity = 1

	check(2, 2)
}

func TestSpotAdmissionAndPerspectiveDepth(t *testing.T) {
	t.Parallel()

	spot := Light{
		Kind: SpotLightKind, Intensity: 1, CastShadows: true,
		Direction: [3]float32{1, 2, 3}, SpotDirection: [3]float32{0, -3, 0},
		GroundColor: [3]float32{6, 0, 0}, InnerAngle: .1, OuterAngle: .5,
	}
	if err := ValidateSpots([]Light{spot}, true); err != nil {
		t.Fatal(err)
	}

	if err := ValidateSpots([]Light{spot}, false); err == nil {
		t.Fatal("accepted spotlight without feature")
	}

	if err := ValidateSpots([]Light{spot, spot, spot}, true); err == nil {
		t.Fatal("accepted a third shadow declaration")
	}

	matrix := spotMatrix(spotPose{spot.Direction, normalized3(spot.SpotDirection), spot.GroundColor[0], spot.OuterAngle})
	for _, test := range []struct {
		y, wantDepth float32
	}{{1.98, 0}, {-4, 1}} {
		center := transformPoint(matrix, [3]float32{1, test.y, 3})

		w := matrix[7]*test.y + matrix[15]
		if absolute(center[0]) > 1e-5 || absolute(center[1]) > 1e-5 || absolute(center[2]/w-test.wantDepth) > 1e-5 {
			t.Fatalf("depth at %v = %v with w %v", test.y, center, w)
		}
	}
}
