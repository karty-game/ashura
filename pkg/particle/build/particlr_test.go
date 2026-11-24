//go:build !js

package build_test

import (
	"bytes"
	"encoding/binary"
	"math"
	"os"
	"strings"
	"testing"

	"github.com/karty-game/ashura/internal/particledata"
	"github.com/karty-game/ashura/pkg/particle"
	"github.com/karty-game/ashura/pkg/particle/build"
)

func TestSpatialLayerCompilation(t *testing.T) {
	t.Parallel()

	source, err := os.ReadFile("../../../samples/particles/assets/ion-plume.prt")
	if err != nil {
		t.Fatal(err)
	}

	var output bytes.Buffer

	settings := build.SpatialLayers{"Cyan jet": {Shape: build.Cone, Radius: 8, SpreadDegrees: 60, HeightOffset: 4}}
	if err := build.Compile3D(&output, bytes.NewReader(source), settings); err != nil {
		t.Fatal(err)
	}

	effect, err := particle.Read(bytes.NewReader(output.Bytes()))
	if err != nil {
		t.Fatal(err)
	}

	for index, layer := range effect.Prepared().Layers {
		var values [4]float32
		for i := range values {
			values[i] = math.Float32frombits(binary.LittleEndian.Uint32(layer.Config[particledata.SpatialOffset+i*4:]))
		}

		var want [4]float32
		if index == 1 {
			want = [4]float32{float32(build.Cone), 8, float32(math.Cos(math.Pi / 6)), 4}
		}

		if values != want {
			t.Fatalf("layer %d settings %v, want %v", index, values, want)
		}
	}

	bad := []build.SpatialLayers{
		{"Typos are errors": {Shape: build.Sphere}},
		{"Blue vapor": {Shape: build.Cone, SpreadDegrees: 181}},
		{"Blue vapor": {Shape: build.GroundPlane, Radius: 2}},
	}
	for _, settings := range bad {
		output.Reset()

		if err := build.Compile3D(&output, bytes.NewReader(source), settings); err == nil || output.Len() != 0 {
			t.Fatalf("spatial settings %v published an invalid bundle", settings)
		}
	}
}

func TestStockPresetsCompile(t *testing.T) {
	t.Parallel()

	for _, name := range []string{"explosion", "smoke"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			//nolint:gosec // Test table holds only fixed fixture names.
			source, err := os.ReadFile("../../../samples/particles/assets/particlr/" + name + ".prt")
			if err != nil {
				t.Fatal(err)
			}

			var output, again bytes.Buffer
			if err := build.Compile(&output, bytes.NewReader(source)); err != nil {
				t.Fatal(err)
			}

			if err := build.Compile(&again, bytes.NewReader(source)); err != nil || !bytes.Equal(again.Bytes(), output.Bytes()) {
				t.Fatalf("repeat build differed: %v", err)
			}

			bundle := output.Bytes()
			if _, err := particle.Read(bytes.NewReader(bundle)); err != nil {
				t.Fatal(err)
			}

			if _, err := particle.Read(bytes.NewReader(bundle[:len(bundle)-1])); err == nil {
				t.Fatal("admitted a truncated texture page")
			}

			var unsupported bytes.Buffer

			changed := strings.ReplaceAll(string(source), `"space": "local"`, `"space": "world"`)
			if err := build.Compile(&unsupported, strings.NewReader(changed)); err == nil || unsupported.Len() != 0 {
				t.Fatal("a world-space document must fail without publishing partial output")
			}
		})
	}
}

func TestPrewarmedWaterfallBuildsDeterministically(t *testing.T) {
	t.Parallel()

	source, err := os.ReadFile("../../../samples/particles/assets/library/waterfall-mist.prt")
	if err != nil {
		t.Fatal(err)
	}

	var output, again bytes.Buffer

	spatial := build.SpatialLayers{"sheet": {Shape: build.Cone, Radius: 36, SpreadDegrees: 11}}
	if err := build.Compile3D(&output, bytes.NewReader(source), spatial); err != nil {
		t.Fatal(err)
	}

	if err := build.Compile3D(&again, bytes.NewReader(source), spatial); err != nil || !bytes.Equal(again.Bytes(), output.Bytes()) {
		t.Fatalf("repeat build differed: %v", err)
	}

	effect, err := particle.Read(&output)
	if err != nil {
		t.Fatal(err)
	}

	sheet, mist, spray := effect.Prepared().Layers[0], effect.Prepared().Layers[1], effect.Prepared().Layers[2]
	if sheet.WarmSlots() == 0 || sheet.WarmLife <= 0 || mist.WarmSlots() == 0 || spray.WarmSlots() == 0 {
		t.Fatal("expected warm particles in the sheet, mist and impact spray")
	}

	if alignment := math.Float32frombits(binary.LittleEndian.Uint32(sheet.Config[particledata.RenderOffset:])); alignment != 2 {
		t.Fatalf("sheet alignment = %v, want velocity", alignment)
	}

	falling := sheet.Warm[particledata.StateHeader:]

	vy := math.Float32frombits(binary.LittleEndian.Uint32(falling[particledata.VelocityOffset+4:]))
	if vy <= 0 {
		t.Fatalf("prewarmed water travels upward, vertical velocity = %v", vy)
	}
}
