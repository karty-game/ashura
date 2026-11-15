//go:build !js

package levelbuild

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/karty-game/karty-webgpu/pkg/assetbuild"
)

func codecFixtureFile() fileRecord {
	return fileRecord{Artifact: assetbuild.Artifact{SHA256: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", Size: 4096}}
}

func TestVariantCodecPlanningSharesExactEncoderInputs(t *testing.T) {
	t.Parallel()

	var plan assetbuild.Plan

	shared := &codecActionCache{}
	file := codecFixtureFile()
	etc := ETC1SOptions{Quality: 192}
	desktop := NativeHDROptions{
		Family:       nativeHDRBCFamily,
		NativeAlbedo: true,
		Encoder: assetbuild.BlockCompressionOptions{

			HDREffort: 2,
		},
	}
	mobile := NativeHDROptions{
		Family:       nativeHDRMobileFamily,
		NativeAlbedo: true,
		Encoder:      assetbuild.BlockCompressionOptions{},
	}

	etcRefs := make([]assetbuild.Ref, 0, 4)
	nativeRefs := make([]assetbuild.Ref, 0, 4)
	ssbumpRefs := make([]assetbuild.Ref, 0, 4)

	for index, profile := range []NativeHDROptions{desktop, mobile, desktop, mobile} {
		identity := fmt.Sprintf("profile%d", index)

		ref, err := addEncodedAlbedo(&plan, identity, "albedo", file, etc, shared)
		if err != nil {
			t.Fatal(err)
		}

		etcRefs = append(etcRefs, ref)

		native, _, err := nativeAlbedoActions(
			&plan,
			identity,
			map[string]fileRecord{"albedo": file},
			profile,

			shared,
		)
		if err != nil {
			t.Fatal(err)
		}

		nativeRefs = append(nativeRefs, native["albedo"])

		ref, err = addNativeBlockAction(&plan, identity, "ssbump", file, profile, false, shared)
		if err != nil {
			t.Fatal(err)
		}

		ssbumpRefs = append(ssbumpRefs, ref)
	}

	for _, ref := range etcRefs {
		if ref != etcRefs[0] {
			t.Fatal("ETC1S repeated across representations/device families", etcRefs)
		}
	}

	if nativeRefs[0] != nativeRefs[2] || nativeRefs[1] != nativeRefs[3] || nativeRefs[0] == nativeRefs[1] {
		t.Fatal("native albedo target family sharing is incorrect", nativeRefs)
	}

	if ssbumpRefs[0] != ssbumpRefs[2] || ssbumpRefs[1] != ssbumpRefs[3] || ssbumpRefs[0] == ssbumpRefs[1] {
		t.Fatal("SSBump native target family sharing is incorrect", ssbumpRefs)
	}
	// One ETC1S + two albedo targets + one shared quality4 UASTC + two
	// SSBump targets. BC6H effort did not duplicate UASTC work.
	if len(shared.refs) != 6 {
		t.Fatal("unexpected unique codec producer count", len(shared.refs))
	}
}

func TestVariantCodecPlanningNeverAliasesDifferentByteInputs(t *testing.T) {
	t.Parallel()

	var plan assetbuild.Plan

	shared := &codecActionCache{}
	file := codecFixtureFile()
	profile := ETC1SOptions{Quality: 192}

	base, err := addEncodedAlbedo(&plan, "base", "albedo", file, profile, shared)
	if err != nil {
		t.Fatal(err)
	}

	for index, change := range []func(*fileRecord, *ETC1SOptions){
		func(file *fileRecord, _ *ETC1SOptions) {
			file.Artifact.SHA256 = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
		},
		func(_ *fileRecord, profile *ETC1SOptions) { profile.Quality++ },
	} {
		source, options := file, profile
		change(&source, &options)

		ref, err := addEncodedAlbedo(&plan, fmt.Sprintf("changed%d", index), "albedo", source, options, shared)
		if err != nil {
			t.Fatal(err)
		}

		if ref == base {
			t.Fatal("distinct codec source/quality aliased", index)
		}
	}
}

type codecCountFixture struct {
	calls            *int
	parallelProfiles []bool
}

func (codecCountFixture) Name() string { return "codec-count-fixture" }
func (fixture codecCountFixture) Plan(plan *assetbuild.Plan) error {
	seed, err := plan.Add(
		assetbuild.Action{
			ID:        "seed",
			Processor: "fixture-source",
			Revision:  "1",
			Outputs:   []assetbuild.Slot{{Name: slot, Schema: "fixture-source@1"}},
			Run: func(ctx *assetbuild.Context) (assetbuild.Result, error) {
				data, err := ctx.ReadFile("source.bin")
				if err != nil {
					return nil, err
				}

				artifact, err := ctx.StoreArtifact(data)
				if err != nil {
					return nil, err
				}

				return result(artifact)
			},
		},
	)
	if err != nil {
		return err
	}

	shared := &codecActionCache{}

	for index, target := range []string{"bc", "etc2", "bc", "etc2"} {
		err := plan.Expand(assetbuild.Expansion{
			ID: fmt.Sprintf("discover-codec-%d", index), Inputs: seed,
			Run: func(ctx *assetbuild.Context, child *assetbuild.Plan) error {
				var source assetbuild.Artifact
				if err := readRecord(ctx, seed[0], &source); err != nil {
					return err
				}

				parallel := index < len(fixture.parallelProfiles) && fixture.parallelProfiles[index]

				return fixture.addProfile(child, shared, source, index, target, parallel)
			},
		})
		if err != nil {
			return err
		}
	}

	return nil
}

func (fixture codecCountFixture) addProfile(
	plan *assetbuild.Plan,
	shared *codecActionCache,
	source assetbuild.Artifact,
	index int,
	target string,
	parallel bool,
) error {
	encoded, err := shared.add(plan, assetbuild.Action{
		Parallel: parallel,
		ID:       fmt.Sprintf("codec-%d", index), Processor: "fixture-codec", Revision: "exact-bytes@1",
		Artifacts: []assetbuild.Artifact{source}, Options: struct {
			Target string `json:"target"`
		}{target}, Outputs: []assetbuild.Slot{{Name: slot, Schema: "fixture-encoded@1"}},
		Run: func(ctx *assetbuild.Context) (assetbuild.Result, error) {
			*fixture.calls++

			data, err := ctx.ReadArtifact(source)
			if err != nil {
				return nil, err
			}

			data = append(data, []byte("/"+target)...)

			output, err := storePublished(ctx, "fixture", ".bin", data)
			if err != nil {
				return nil, err
			}

			return result(output)
		},
	})
	if err != nil {
		return err
	}

	outputs, err := plan.Add(assetbuild.Action{
		ID: fmt.Sprintf("publish-%d", index), Processor: "fixture-consumer", Revision: "1",
		Inputs: encoded, Outputs: []assetbuild.Slot{{Name: filesSlot, Schema: filesSchema}},
		Run: func(ctx *assetbuild.Context) (assetbuild.Result, error) {
			var file fileRecord
			if err := readRecord(ctx, encoded[0], &file); err != nil {
				return nil, err
			}

			data, err := ctx.ReadArtifact(file.Artifact)
			if err != nil {
				return nil, err
			}

			bundle, err := assetbuild.EncodeBundle(assetbuild.Bundle{fmt.Sprintf("profile-%d.bin", index): data})
			if err != nil {
				return nil, err
			}

			return assetbuild.Result{filesSlot: bundle}, nil
		},
	})
	if err != nil {
		return err
	}

	plan.Publish(outputs[0])

	return nil
}

func TestSharedCodecProducerExecutesOnceAndPreservesConsumerProvenance(t *testing.T) {
	t.Parallel()

	project := t.TempDir()
	if err := os.WriteFile(filepath.Join(project, "source.bin"), []byte("source"), 0o600); err != nil {
		t.Fatal(err)
	}

	calls := 0
	config := assetbuild.Config{Output: "generated",
		Pipelines: []assetbuild.Pipeline{{Name: "fixture",
			Input:  []string{"source.bin"},
			Import: []assetbuild.Plugin{codecCountFixture{calls: &calls}},
		}},
	}
	options := assetbuild.Options{Project: project, Identity: "shared-codec-fixture@1"}

	report, err := assetbuild.Build(t.Context(), config, options)
	if err != nil {
		t.Fatal(err)
	}

	if calls != 2 {
		t.Fatal("identical producer calls repeated", calls, report.Actions)
	}

	for index, target := range []string{"bc", "etc2", "bc", "etc2"} {
		//nolint:gosec // Fixed numeric fixture names remain inside this test's private TempDir.
		data, err := os.ReadFile(
			filepath.Join(project, "generated", fmt.Sprintf("profile-%d.bin", index)),
		)
		if err != nil || string(data) != "source/"+target {
			t.Fatal("consumer received different codec bytes", index, string(data), err)
		}
	}

	// Change which equivalent requester opts in first. Execution metadata must
	// neither duplicate producers nor invalidate their admitted cache/provenance.
	options.Jobs = 4
	config.Pipelines[0].Import = []assetbuild.Plugin{codecCountFixture{
		calls: &calls, parallelProfiles: []bool{true, false, false, true},
	}}

	again, err := assetbuild.Build(t.Context(), config, options)
	if err != nil || calls != 2 || again.Executed != 0 {
		t.Fatal("unchanged shared build performed codec work", calls, again, err)
	}

	config.Pipelines[0].Import = []assetbuild.Plugin{codecCountFixture{
		calls: &calls, parallelProfiles: []bool{false, true, true, false},
	}}

	again, err = assetbuild.Build(t.Context(), config, options)
	if err != nil || calls != 2 || again.Executed != 0 {
		t.Fatal("mixed requester execution order changed cache identity", calls, again, err)
	}

	if err := os.WriteFile(filepath.Join(project, "source.bin"), []byte("changed"), 0o600); err != nil {
		t.Fatal(err)
	}

	if _, err := assetbuild.Build(t.Context(), config, options); err != nil {
		t.Fatal(err)
	}

	if calls != 4 {
		t.Fatal("source change did not rerun exactly two distinct conversions", calls)
	}
}
