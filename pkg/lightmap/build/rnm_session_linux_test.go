//go:build linux

package lightmapbuild_test

import (
	"bytes"
	"context"
	"os"
	"testing"
	"time"

	"github.com/karty-game/ashura/internal/lightmapdata"
	"github.com/karty-game/ashura/internal/lightmapfixture"
	"github.com/karty-game/ashura/pkg/lightmap"
	lightmapbuild "github.com/karty-game/ashura/pkg/lightmap/build"
)

func TestHardwareRNMSessionReusesWorkerAndMatchesOneShot(t *testing.T) {
	t.Parallel()

	if os.Getenv("KARTY_LIGHTMAP_GPU_TEST") != "1" {
		t.Skip("set KARTY_LIGHTMAP_GPU_TEST=1 with hardware GPU access")
	}

	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()

	options := lightmapbuild.BrowserOptions{Executable: os.Getenv("KARTY_LIGHTMAP_BROWSER")}

	session, err := lightmapbuild.NewRNMSession(ctx, options)
	if err != nil {
		t.Fatal(err)
	}

	defer func() { _ = session.Close() }()

	document := lightmapfixture.TransportDocument()

	encoded, err := lightmapdata.Encode(document)
	if err != nil {
		t.Fatal(err)
	}

	asset, err := lightmap.Decode(encoded)
	if err != nil {
		t.Fatal(err)
	}

	scene := lightmap.RNMScene{SkyColor: [3]float32{.4, .3, .2}, GroundColor: [3]float32{.4, .3, .2}, Bounces: 1, RowsPerRecord: 1}

	first, err := session.Bake(ctx, asset, scene)
	if err != nil {
		t.Fatal(err)
	}

	same, err := session.Bake(ctx, asset, scene)
	if err != nil {
		t.Fatal(err)
	}

	reference, err := lightmapbuild.BakeRNMBrowser(ctx, asset, scene, options)
	if err != nil {
		t.Fatal(err)
	}

	for index := range first.Textures {
		if !bytes.Equal(first.Textures[index].Levels[0], same.Textures[index].Levels[0]) ||
			!bytes.Equal(first.Textures[index].Levels[0], reference.Textures[index].Levels[0]) {
			t.Fatalf("session result differs from shared one-shot kernel, map%d", index)
		}
	}

	scene.SkyColor = [3]float32{.1, .2, .6}
	scene.GroundColor = scene.SkyColor

	changed, err := session.Bake(ctx, asset, scene)
	if err != nil {
		t.Fatal(err)
	}

	if bytes.Equal(changed.Textures[0].Levels[0], first.Textures[0].Levels[0]) {
		t.Fatal("changed job reused stale outputs")
	}

	if first.Adapter == "" || first.Adapter != same.Adapter || first.Scale != reference.Scale || first.Encoding != reference.Encoding ||
		first.Samples != reference.Samples {
		t.Fatal("session metadata mismatch")
	}

	t.Logf("adapter=%s firstjob=%s nextjob=%s changedjob=%s", first.Adapter, first.Duration, same.Duration, changed.Duration)

	if err = session.Close(); err != nil {
		t.Fatal(err)
	}

	if _, err = session.Bake(ctx, asset, scene); err == nil {
		t.Fatal("closed GPU worker accepted a job")
	}
}
