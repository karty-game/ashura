package lightmap_test

import (
	"testing"

	"github.com/karty-game/karty-webgpu/internal/bufferdata"
	"github.com/karty-game/karty-webgpu/internal/computedata"
	"github.com/karty-game/karty-webgpu/internal/lightmapdata"
	"github.com/karty-game/karty-webgpu/internal/lightmapfixture"
	"github.com/karty-game/karty-webgpu/internal/texturedata"
	"github.com/karty-game/karty-webgpu/pkg/compute"
	"github.com/karty-game/karty-webgpu/pkg/lightmap"
	"github.com/karty-game/karty-webgpu/pkg/shader"
	"github.com/karty-game/karty-webgpu/pkg/texture"
)

type owner struct {
	buffers    []*shader.Buffer
	dispatches int
}

func (o *owner) MarkBufferDirty(_ *bufferdata.Buffer) {}
func (o *owner) NewBuffer(desc shader.BufferDesc) (*shader.Buffer, error) {
	buffer, err := bufferdata.Create(o, desc)
	if err == nil {
		o.buffers = append(o.buffers, buffer)
	}

	return buffer, err
}
func (o *owner) NewComputePipeline(desc compute.PipelineDesc) (*compute.Pipeline, error) {
	return computedata.NewPipeline(o, desc)
}
func (o *owner) NewStorageTexture(width, height int) (*texture.Texture, error) {
	return texturedata.CreateStorage(width, height)
}
func (o *owner) NewComputeBindings(pipeline *compute.Pipeline, bindings []shader.Binding) (*compute.BindGroup, error) {
	return computedata.NewBindGroup(pipeline, bindings), nil
}
func (o *owner) Dispatch(_ *compute.Pipeline, _ *compute.BindGroup, x, y, z uint32) error {
	if x != 1 || y != 1 || z != 1 {
		panic("unexpected fixture workgroups")
	}

	o.dispatches++

	return nil
}
func (o *owner) uploads() int {
	var pending []bufferdata.Upload
	for _, buffer := range o.buffers {
		pending = bufferdata.Pending(buffer, pending)
	}

	return len(pending)
}

func TestOnlyRequestedChangedLightingSchedulesWork(t *testing.T) {
	t.Parallel()

	encoded, err := lightmapdata.Encode(lightmapfixture.Document())
	if err != nil {
		t.Fatal(err)
	}

	asset, err := lightmap.Decode(encoded)
	if err != nil {
		t.Fatal(err)
	}

	resources := &owner{}

	baker, err := lightmap.New(resources, asset)
	if err != nil {
		t.Fatal(err)
	}

	if err = baker.Record(resources); err != nil || resources.dispatches != 0 {
		t.Fatal("unrequested bake dispatched")
	}

	_ = resources.uploads()

	point := lightmap.Point{Position: [3]float32{0, 2, 0}, Color: [3]float32{1, .5, 0}, Intensity: 2, Range: 10}
	if err = baker.SetPoint(point); err != nil {
		t.Fatal(err)
	}

	if resources.uploads() != 1 || !baker.Pending() || baker.Scale() != 2 {
		t.Fatal("source update did not write just parameters")
	}

	if err = baker.Record(resources); err != nil {
		t.Fatal(err)
	}

	if resources.dispatches != 1 || baker.Recordings() != 1 || baker.Pending() {
		t.Fatal("requested bake not recorded once")
	}

	for range 10 {
		if err = baker.SetPoint(point); err != nil {
			t.Fatal(err)
		}

		if err = baker.Record(resources); err != nil {
			t.Fatal(err)
		}
	}

	if resources.uploads() != 0 || resources.dispatches != 1 {
		t.Fatal("unchanged lighting produced GPU work")
	}

	point.Position[0]++
	if err = baker.SetPoint(point); err != nil {
		t.Fatal(err)
	}

	if err = baker.Record(resources); err != nil {
		t.Fatal(err)
	}

	if resources.uploads() != 1 || resources.dispatches != 2 {
		t.Fatal("changed lighting did not request another bake")
	}

	point.Range = -1
	if err = baker.SetPoint(point); err == nil || baker.Pending() {
		t.Fatal("invalid source changed bake state")
	}
}
