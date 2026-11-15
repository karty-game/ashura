package render_test

import (
	"encoding/binary"
	"math"
	"testing"

	"github.com/karty-game/karty-webgpu/internal/render"
)

func pointFixture() render.Light {
	return render.Light{
		Kind:             render.PointLightKind,
		Direction:        [3]float32{2, 3, 4},
		Color:            [3]float32{1, .7, .4},
		GroundColor:      [3]float32{20, 1, 0},
		Intensity:        1,
		CastShadows:      true,
		ShadowResolution: 512,
		ShadowBias:       .015,
	}
}

func TestPointShadowAdmission(t *testing.T) {
	t.Parallel()

	good := pointFixture()
	if err := render.ValidatePointLights([]render.Light{good}); err != nil {
		t.Fatal(err)
	}

	for _, change := range []func(*render.Light){func(l *render.Light) { l.GroundColor[0] = 0 }, func(l *render.Light) { l.ShadowResolution = 100 }, func(l *render.Light) { l.ShadowResolution = 4096 }, func(l *render.Light) { l.ShadowBias = -1 }, func(l *render.Light) { l.Direction[0] = float32(math.NaN()) }, func(l *render.Light) { l.Intensity = -1 }} {
		bad := good
		change(&bad)

		if render.ValidatePointLights([]render.Light{bad}) == nil {
			t.Fatal("invalid point light admitted")
		}
	}

	if render.ValidatePointLights([]render.Light{good, good}) == nil {
		t.Fatal("multiple shadow points silently admitted")
	}

	disabled := good

	disabled.Intensity = 0
	if err := render.ValidatePointLights([]render.Light{good, disabled}); err != nil {
		t.Fatal(err)
	}
}

func TestPointShadowCacheTracksCastersAndIgnoresCameraAndRadiance(t *testing.T) {
	t.Parallel()

	light := pointFixture()
	frame := render.Frame{Draws: []render.DrawCall{{ID: 1, Vertices: []render.Vertex{{}, {}, {}}, Indices: []uint32{0, 1, 2}}}}

	var state render.PointShadowState
	if !state.Update(frame, light) || state.Update(frame, light) {
		t.Fatal("first/unchanged shadow cache incorrect")
	}

	frame.ViewProjection[0] = 2
	frame.CameraPosition[0] = 10
	light.Intensity = 3
	light.Color[0] = .2

	light.ShadowBias = .03
	if state.Update(frame, light) {
		t.Fatal("camera/radiance/comparison-only changes invalidate cached depth")
	}

	frame.Draws[0].Transform[12] = 1
	if !state.Update(frame, light) {
		t.Fatal("moving caster did not invalidate")
	}

	frame.Draws[0].InstanceVersion++
	if !state.Update(frame, light) {
		t.Fatal("instance change did not invalidate")
	}

	light.Direction[2]++
	if !state.Update(frame, light) {
		t.Fatal("moving light did not invalidate")
	}

	frame.Draws = nil
	if !state.Update(frame, light) {
		t.Fatal("removed caster left stale shadow")
	}

	if state.Update(frame, light) {
		t.Fatal("empty unchanged scene repeatedly invalidates")
	}

	state.Invalidate()

	if !state.Update(frame, light) {
		t.Fatal("recreated depth was treated as populated")
	}
}

func TestPointShadowFacesProjectOntoCubeConvention(t *testing.T) {
	t.Parallel()

	light := pointFixture()

	directions := [6][3]float32{{1, 0, 0}, {-1, 0, 0}, {0, 1, 0}, {0, -1, 0}, {0, 0, 1}, {0, 0, -1}}
	for face, dir := range directions {
		packed := render.PointShadowFaceUniform(light, face)

		var matrix [16]float32
		for i := range matrix {
			matrix[i] = math.Float32frombits(binary.LittleEndian.Uint32(packed[i*4:]))
		}

		project := func(p [3]float32) [3]float32 {
			var clip [4]float32
			for row := range 4 {
				clip[row] = matrix[12+row]
				for col := range 3 {
					clip[row] += matrix[col*4+row] * p[col]
				}
			}

			return [3]float32{clip[0] / clip[3], clip[1] / clip[3], clip[2] / clip[3]}
		}

		center := light.Direction
		for i := range 3 {
			center[i] += dir[i] * 5
		}

		clip := project(center)
		if math.Abs(float64(clip[0])) > 1e-4 || math.Abs(float64(clip[1])) > 1e-4 || clip[2] <= 0 || clip[2] >= 1 {
			t.Fatalf("face %d center projection=%v", face, clip)
		}
		// All four corners of a face must map to their cube texture directions.
		for _, coordinate := range [][2]float32{{-.5, -.5}, {-.5, .5}, {.5, -.5}, {.5, .5}} {
			var direction [3]float32

			switch face {
			case 0:
				direction = [3]float32{1, -coordinate[1], -coordinate[0]}
			case 1:
				direction = [3]float32{-1, -coordinate[1], coordinate[0]}
			case 2:
				direction = [3]float32{coordinate[0], 1, coordinate[1]}
			case 3:
				direction = [3]float32{coordinate[0], -1, -coordinate[1]}
			case 4:
				direction = [3]float32{coordinate[0], -coordinate[1], 1}
			case 5:
				direction = [3]float32{-coordinate[0], -coordinate[1], -1}
			}

			p := light.Direction
			for i := range 3 {
				p[i] += direction[i] * 5
			}

			clip = project(p)
			if math.Abs(float64(clip[0]-coordinate[0])) > 1e-4 || math.Abs(float64(-clip[1]-coordinate[1])) > 1e-4 {
				t.Fatalf("face %d cube coordinate %v projected %v", face, coordinate, clip)
			}
		}
	}
}

func TestPointUniformStateTracksRangeAndSourcePosition(t *testing.T) {
	t.Parallel()

	for _, source := range []bool{false, true} {
		draw := render.DrawCall{Source: source}
		frame := render.Frame{Lights: []render.Light{pointFixture()}}

		var state render.UniformState
		state.Update(draw, frame)

		frame.Lights[0].GroundColor[0] = 10
		if !state.Update(draw, frame) {
			t.Fatalf("source=%v range change did not upload", source)
		}

		frame.Lights[0].Direction[1]++
		if !state.Update(draw, frame) {
			t.Fatalf("source=%v position change did not upload", source)
		}
	}
}
