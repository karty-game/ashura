//go:build !js

package levelbuild

import (
	"fmt"
	"testing"

	"github.com/karty-game/ashura/pkg/assetbuild"
)

func TestPagePackagingSharesOnlyIdenticalProfileInputs(t *testing.T) {
	t.Parallel()

	var plan assetbuild.Plan

	shared := &packageActionCache{}
	baseOptions := pagePackagingOptions{Page: pageRecord{Width: 512, Height: 512, Representation: "rnm-fit-v1"}}
	base := assetbuild.Action{
		ID: "bc-page", Processor: "level-package-page", Revision: "shared-profile-pages@2",
		Artifacts: []assetbuild.Artifact{codecFixtureFile().Artifact},
		Options:   baseOptions,
		Outputs:   []assetbuild.Slot{{Name: slot, Schema: "karty.level-page@1"}},
		Run:       func(*assetbuild.Context) (assetbuild.Result, error) { return result(publishedPage{}) },
	}

	first, err := shared.add(&plan, base)
	if err != nil {
		t.Fatal(err)
	}

	other := base
	other.ID, other.Parallel = "mobile-page", true

	repeated, err := shared.add(&plan, other)
	if err != nil || repeated[0] != first[0] {
		t.Fatal("device profiles repeated the same geometry/material packaging", err)
	}

	for index, change := range []func(*pagePackagingOptions){
		func(options *pagePackagingOptions) { options.Origin[0] = 1 },
		func(options *pagePackagingOptions) { options.Development = true },
		func(options *pagePackagingOptions) { options.Page.Representation = "rgb-diffuse-v1" },
		func(options *pagePackagingOptions) {
			options.Page.SSBump = map[string]assetbuild.Artifact{"changed": codecFixtureFile().Artifact}
		},
	} {
		changed := base
		options := baseOptions

		change(&options)
		changed.ID, changed.Options = fmt.Sprintf("changed-%d", index), options

		refs, addErr := shared.add(&plan, changed)
		if addErr != nil || refs[0] == first[0] {
			t.Fatal("different page/material/origin options aliased", index, addErr)
		}
	}

	changed := base
	changed.Artifacts = []assetbuild.Artifact{{SHA256: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", Size: 4096}}

	refs, err := shared.add(&plan, changed)
	if err != nil || refs[0] == first[0] {
		t.Fatal("different artifact admission aliased", err)
	}
}
