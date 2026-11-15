//go:build !js

package levelbuild

import (
	"github.com/karty-game/karty-webgpu/internal/leveldata"
	"github.com/karty-game/karty-webgpu/pkg/assetbuild"
)

func selectPublishedRepresentation(page pageRecord, representation string) (pageRecord, error) {
	if representation == "" {
		return page, nil
	}

	switch representation {
	case leveldata.RNMFit:
		if page.Representation != leveldata.RNMFit {
			return page, ErrInvalid
		}
	case leveldata.RGBDiffuse:
		if page.RGBMap.SHA256 == "" {
			return page, ErrInvalid
		}

		page.Representation = representation
		page.Maps = [3]assetbuild.Artifact{page.RGBMap}
		page.SSBump = nil
	default:
		return page, ErrInvalid
	}

	return page, nil
}
