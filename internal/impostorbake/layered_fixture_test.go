//go:build !js

package impostorbake

import (
	"testing"

	"github.com/karty-game/karty-webgpu/internal/impostordata"
	"github.com/karty-game/karty-webgpu/internal/impostorfixture"
	"github.com/karty-game/karty-webgpu/internal/modelbuild"
	"github.com/karty-game/karty-webgpu/internal/modeldata"
)

func TestLayeredAuthoredTreeReportsOnlyTinyKnownPocket(t *testing.T) {
	t.Parallel()

	source, _, err := impostorfixture.Tree()
	if err != nil {
		t.Fatal(err)
	}

	compiled, err := modelbuild.Compile(source, "tree.glb")
	if err != nil {
		t.Fatal(err)
	}

	document, err := modeldata.Decode(compiled.Data)
	if err != nil {
		t.Fatal(err)
	}

	captured, report, err := BakeWithReport(
		t.Context(),
		document,
		Options{Mode: impostordata.ModeLayered, Resolution: 128, MaxIntervals: 8},
	)
	if err != nil {
		t.Fatal(err)
	}

	if report.MaximumIntervals != 8 || report.ToleratedNegativePockets != 1 || report.MaximumNegativePocketDepth <= 0 ||
		report.MaximumNegativePocketDepth > float64(captured.Radius)*1e-4 {
		t.Fatalf("unexpected source geometry diagnostics %+v", report)
	}

	t.Logf("layered source diagnostics %+v", report)
}
