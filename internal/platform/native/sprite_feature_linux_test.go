//go:build linux

package native

import (
	"errors"
	"testing"

	"github.com/karty-game/ashura/internal/platform"
	"github.com/karty-game/ashura/internal/render"
	"github.com/karty-game/ashura/internal/spritepaint"
	wgpu "github.com/karty-game/ashura/internal/webgpu"
)

func TestUnregisteredSpritesRejectDrawsButAllowEmptyViewport(t *testing.T) {
	t.Parallel()

	driver := &Driver{}

	frame := &spritepaint.Frame{Batches: []spritepaint.Batch{{Count: 1}}}
	if err := driver.updateSprites(frame); !errors.Is(err, platform.ErrFeatureMissing) {
		t.Fatalf("unregistered sprites were accepted: %v", err)
	}

	for _, frame := range []*spritepaint.Frame{nil, {Width: 320, Height: 240}} {
		if err := driver.updateSprites(frame); err != nil {
			t.Fatalf("empty viewport required sprite implementation: %v", err)
		}
	}
}

type mockShadowFeature struct {
	frame          render.Frame
	records, draws int
}

func (backend *mockShadowFeature) Record(_ *wgpu.CommandEncoder, frame render.Frame) error {
	backend.frame = frame
	backend.records++

	return nil
}

func (backend *mockShadowFeature) DrawDebug(_ *wgpu.RenderPassEncoder, _ render.Frame) {
	backend.draws++
}

func (*mockShadowFeature) Close() {}

func TestRegisteredPointShadowHooks(t *testing.T) {
	t.Parallel()

	backend := &mockShadowFeature{}
	driver := &Driver{shadowBridge: backend}

	frame := shadowFeatureFrame(128)
	if err := driver.recordPointShadow(nil, frame); err != nil {
		t.Fatal(err)
	}

	driver.drawPointShadowDebug(nil, frame)

	if backend.records != 1 || backend.draws != 1 || len(backend.frame.Lights) != 1 {
		t.Fatalf("registered shadow hooks were not called: %+v", backend)
	}
}

func TestPointShadowFeatureCloseKeepsSamplingOwner(t *testing.T) {
	t.Parallel()

	shared := &gpuPointShadow{}
	driver := &Driver{pointShadow: shared}
	feature := &pointShadowFeature{Driver: driver, pointShadow: &pointShadowGPU{gpuPointShadow: shared}}
	feature.Close()
	feature.Close()

	if feature.pointShadow != nil || driver.pointShadow != shared {
		t.Fatal("optional shadow close changed shared sampling ownership")
	}
}

type mockSpriteFeature struct {
	frame                  *spritepaint.Frame
	updates, draws, closes int
}

func (backend *mockSpriteFeature) Update(frame *spritepaint.Frame) error {
	backend.frame = frame
	backend.updates++

	return nil
}

func (backend *mockSpriteFeature) Draw(_ *wgpu.CommandEncoder, _ *wgpu.TextureView) error {
	backend.draws++

	return nil
}

func (backend *mockSpriteFeature) Close() { backend.closes++ }

func TestRegisteredSpriteHooksAndCloseOwnership(t *testing.T) {
	t.Parallel()

	backend := &mockSpriteFeature{}
	driver := &Driver{spriteBridge: backend}

	frame := &spritepaint.Frame{Batches: []spritepaint.Batch{{Count: 1}}}
	if err := driver.updateSprites(frame); err != nil {
		t.Fatal(err)
	}

	if err := driver.drawSprites(nil, nil); err != nil {
		t.Fatal(err)
	}

	driver.closeSprites()
	driver.closeSprites()

	if backend.frame != frame || backend.updates != 1 || backend.draws != 1 || backend.closes != 1 || driver.spriteBridge != nil {
		t.Fatalf("sprite hooks/lifecycle not preserved: %+v", backend)
	}
}

func TestUnregisteredPointShadowRequiresFeature(t *testing.T) {
	t.Parallel()

	driver := &Driver{}

	frame := shadowFeatureFrame(128)
	if err := driver.recordPointShadow(nil, frame); !errors.Is(err, platform.ErrFeatureMissing) {
		t.Fatalf("unregistered shadow was accepted: %v", err)
	}

	if err := driver.recordPointShadow(nil, render.Frame{}); err != nil {
		t.Fatalf("plain scene required shadow implementation: %v", err)
	}
}

func TestPointShadowSamplingSetupReusesMatchingResources(t *testing.T) {
	t.Parallel()

	for _, resolution := range []uint32{0, 128} {
		stored := resolution
		if stored == 0 {
			stored = 1
		}

		shared := &gpuPointShadow{resolution: stored}
		driver := &Driver{standardPipeline: &wgpu.RenderPipeline{}, pointShadow: shared}
		// No device exists. Any accidental allocation or heavy pass setup fails.
		if err := driver.preparePointShadowSampling(resolution); err != nil || driver.pointShadow != shared {
			t.Fatalf("sampling setup did not reuse its resources: %v", err)
		}
	}
}

func TestPointShadowResolutionStillRequiresStartupPreparation(t *testing.T) {
	t.Parallel()

	feature := &pointShadowFeature{pointShadow: &pointShadowGPU{
		gpuPointShadow: &gpuPointShadow{resolution: 128},
		pipelines:      [2]*wgpu.RenderPipeline{{}},
	}}
	if err := feature.Record(nil, shadowFeatureFrame(256)); !errors.Is(err, render.ErrPointShadowStartup) {
		t.Fatalf("changed resolution did not require startup preparation: %v", err)
	}
}

func shadowFeatureFrame(resolution uint32) render.Frame {
	return render.Frame{Lights: []render.Light{{
		Kind: render.PointLightKind, CastShadows: true, Intensity: 1, ShadowResolution: resolution,
	}}}
}
