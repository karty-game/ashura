package render_test

import (
	"testing"

	"github.com/karty-game/karty-webgpu/internal/render"
)

func TestInstanceDirtyRanges(t *testing.T) {
	t.Parallel()

	draw := render.DrawCall{Instances: make([]render.InstanceTransform, 5)}

	var state render.InstanceState
	state.Update(draw)

	if len(state.Dirty) != 1 || state.Dirty[0] != (render.Span{Offset: 0, End: 5 * render.InstanceStride}) {
		t.Fatalf("initial ranges: %v", state.Dirty)
	}

	state.Update(draw)

	if len(state.Dirty) != 0 {
		t.Fatalf("unchanged matrices uploaded: %v", state.Dirty)
	}

	draw.Instances[1].Model[12] = 2
	draw.Instances[3].Model[13] = 4
	state.Update(draw)

	want := []render.Span{
		{Offset: render.InstanceStride, End: 2 * render.InstanceStride},
		{Offset: 3 * render.InstanceStride, End: 4 * render.InstanceStride},
	}
	if len(state.Dirty) != len(want) || state.Dirty[0] != want[0] || state.Dirty[1] != want[1] {
		t.Fatalf("sparse matrix edit spans=%v want%v", state.Dirty, want)
	}

	draw.Instances = draw.Instances[:2]
	state.Update(draw)

	if state.Count() != 2 || len(state.Dirty) != 0 {
		t.Fatalf("shrinking unchanged matrices uploads data: count=%d spans=%v", state.Count(), state.Dirty)
	}

	draw.Instances = append(draw.Instances, render.InstanceTransform{})
	state.Update(draw)

	if len(state.Dirty) != 1 || state.Dirty[0].Offset != 2*render.InstanceStride {
		t.Fatalf("regrown matrix was not uploaded: %v", state.Dirty)
	}
}

func TestUniformVisibilityChanges(t *testing.T) {
	t.Parallel()

	var state render.UniformState

	draw := render.DrawCall{Color: [4]float32{1, 1, 1, 1}, Roughness: .5}

	frame := render.Frame{Lights: []render.Light{{Kind: render.DirectionalLightKind, Intensity: 1}}}
	if !state.Update(draw, frame) || state.Update(draw, frame) {
		t.Fatal("initial/unchanged uniform state incorrect")
	}

	draw.Transform[12] = 3
	if state.Update(draw, frame) {
		t.Fatal("model matrix invalidated material uniforms")
	}

	frame.CameraPosition[0] = 1
	if !state.Update(draw, frame) {
		t.Fatal("camera-aware standard material failed to update")
	}

	frame.Lights[0].Intensity = .5
	if !state.Update(draw, frame) {
		t.Fatal("light mutation failed to update")
	}

	draw.Source = true
	if !state.Update(draw, frame) {
		t.Fatal("source material replacement failed to update")
	}

	frame.CameraPosition[0] = 2
	draw.Roughness = .2

	draw.AOStrength = .9
	if state.Update(draw, frame) {
		t.Fatal("unused standard shading fields invalidated Source uniforms")
	}

	draw.AmbientColors[0][0] = .3
	if !state.Update(draw, frame) {
		t.Fatal("Source ambient mutation failed to update")
	}
}

//nolint:paralleltest // AllocsPerRun must be isolated on Go 1.27.
func TestChangedInstanceAndUniformStateDoNotAllocate(t *testing.T) {
	draw := render.DrawCall{Instances: make([]render.InstanceTransform, 1000)}
	frame := render.Frame{Lights: make([]render.Light, 8)}

	var (
		instances render.InstanceState
		uniforms  render.UniformState
	)

	instances.Update(draw)
	uniforms.Update(draw, frame)

	allocations := testing.AllocsPerRun(100, func() {
		draw.Instances[500].Model[12]++
		frame.CameraPosition[0]++

		instances.Update(draw)
		uniforms.Update(draw, frame)
	})
	if allocations != 0 {
		t.Fatalf("warm dirty updates allocated %v times", allocations)
	}
}

func TestTrustedInstanceRevisionsAndSkippedFrames(t *testing.T) {
	t.Parallel()

	draw := render.DrawCall{Instances: make([]render.InstanceTransform, 4), InstanceOwner: new(int), InstanceVersion: 1}

	var state render.InstanceState
	state.Update(draw)
	draw.Instances[1].Model[12] = 1
	draw.ChangedInstances = []int{1}
	draw.InstanceVersion = 2
	state.Update(draw)

	if len(state.Dirty) != 1 || state.Dirty[0] != (render.Span{Offset: 64, End: 128}) {
		t.Fatalf("one changed record uploaded more than64B: %v", state.Dirty)
	}

	state.Update(draw)

	if len(state.Dirty) != 0 {
		t.Fatal("unchanged revision reuploaded instance data")
	}
	// Revision3 was assembled while presentation was unavailable. Revision4 only
	// names its own edit; the backend must recover all changes since revision2.
	draw.Instances[2].Model[12] = 2
	draw.Instances[3].Model[12] = 3
	draw.ChangedInstances = []int{3}
	draw.InstanceVersion = 4
	state.Update(draw)

	if len(state.Dirty) != 1 || state.Dirty[0] != (render.Span{Offset: 128, End: 256}) {
		t.Fatalf("skipped submission lost an earlier edit: %v", state.Dirty)
	}

	draw.InstanceOwner = new(int)
	draw.Instances[0].Model[12] = 4
	state.Update(draw)

	if len(state.Dirty) != 1 || state.Dirty[0].Offset != 0 {
		t.Fatal("different world/batch owner reused a stale revision")
	}
}

func TestUnusedLightFieldsDoNotInvalidateUniforms(t *testing.T) {
	t.Parallel()

	frame := render.Frame{Lights: []render.Light{{Kind: render.DirectionalLightKind}, {Kind: render.HemisphereLightKind}}}

	var state render.UniformState

	draw := render.DrawCall{}
	state.Update(draw, frame)

	frame.Lights[0].GroundColor[0] = .5
	if state.Update(draw, frame) {
		t.Fatal("Standard directional ground color triggered an upload")
	}

	draw.Source = true
	state.Update(draw, frame)
	frame.Lights[0].GroundColor[0] = .7
	frame.Lights[1].GroundColor[1] = .4
	frame.Lights[1].Color[0] = .8
	frame.Lights[1].Intensity = 2

	frame.Lights[1].Direction[2] = 1
	if state.Update(draw, frame) {
		t.Fatal("Source shader-unused hemisphere/ground fields triggered an upload")
	}

	frame.Lights[0].Intensity = 3
	if !state.Update(draw, frame) {
		t.Fatal("Source directional intensity mutation was lost")
	}
}
