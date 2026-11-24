package engine

import (
	"errors"
	"testing"

	"github.com/karty-game/ashura/internal/render"
)

func TestClusteredFrameAdmitsSixtyFourLights(t *testing.T) {
	t.Parallel()

	world := NewWorld()
	for range render.ClusterLightLimit {
		world.Add(&namedLightFixture{LightData{Kind: PointLight, Range: 2, Intensity: 1}})
	}

	frame, err := makeFrameWithLighting(world, 640, 480, true)
	if err != nil || len(frame.Lights) != render.ClusterLightLimit {
		t.Fatalf("frame has %d lights: %v", len(frame.Lights), err)
	}

	if _, err := makeFrame(world, 640, 480); !errors.Is(err, errTooManyLights) {
		t.Fatalf("legacy light limit: %v", err)
	}

	world.Add(&namedLightFixture{LightData{Kind: PointLight, Range: 2, Intensity: 1}})

	if _, err := makeFrameWithLighting(world, 640, 480, true); !errors.Is(err, errClusterLightCapacity) {
		t.Fatalf("clustered light limit: %v", err)
	}
}
