//go:build !js

package levelbuild

import (
	"encoding/json"

	"github.com/karty-game/karty-webgpu/pkg/assetbuild"
)

func readSSBump(ctx *assetbuild.Context, resource assetbuild.Resource, schema string) (assetbuild.Artifact, error) {
	if schema != assetbuild.PreparedSetSchema && schema != assetbuild.ImageSetSchema {
		return assetbuild.Artifact{}, ErrInvalid
	}

	data, err := ctx.Input(resource.Ref)
	if err != nil {
		return assetbuild.Artifact{}, err
	}

	if schema == assetbuild.ImageSetSchema {
		return storeReliefImages(ctx, data)
	}

	var set assetbuild.PreparedSet
	if err = json.Unmarshal(data, &set); err != nil {
		return assetbuild.Artifact{}, err
	}

	var selected []byte

	for _, image := range set.Images {
		if image.Usage == assetbuild.UsageSSBump {
			if selected != nil {
				return assetbuild.Artifact{}, ErrInvalid
			}

			selected = image.Data
		}
	}

	if selected == nil {
		return assetbuild.Artifact{}, ErrInvalid
	}

	return ctx.StoreArtifact(selected)
}

func storeReliefImages(ctx *assetbuild.Context, data []byte) (assetbuild.Artifact, error) {
	set, err := assetbuild.DecodeImageSet(data)
	if err != nil {
		return assetbuild.Artifact{}, err
	}

	var selected *assetbuild.Image

	for index := range set.Images {
		value := &set.Images[index]
		if value.Usage != assetbuild.UsageSSBump {
			continue
		}

		if selected != nil {
			return assetbuild.Artifact{}, ErrInvalid
		}

		selected = value
	}

	if selected == nil {
		return assetbuild.Artifact{}, ErrInvalid
	}

	encoded, err := assetbuild.EncodeTexture(*selected)
	if err != nil {
		return assetbuild.Artifact{}, err
	}

	return ctx.StoreArtifact(encoded)
}
