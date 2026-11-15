package level

import (
	"errors"
	"testing"

	"github.com/karty-game/karty-webgpu/pkg/scene/resource"
	"github.com/karty-game/karty-webgpu/pkg/texture"
)

func TestSelectPreparedProfileBeforeIO(t *testing.T) {
	t.Parallel()

	variants := TextureVariants{BC: resource.Asset{Path: "bc-manifest"}, Mobile: resource.Asset{Path: "mobile-manifest"}}

	bc, err := variants.Select(texture.BC1)
	if err != nil || bc != variants.BC {
		t.Fatal(bc, err)
	}

	mobile, err := variants.Select(texture.ETC2RGB)
	if err != nil || mobile != variants.Mobile {
		t.Fatal(mobile, err)
	}

	empty, err := variants.Select(texture.RGBA8)
	if !errors.Is(err, ErrNativeHDR) || empty != (resource.Asset{}) {
		t.Fatal(empty, err)
	}
}
