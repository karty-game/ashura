//go:build !js

package modelbuild

import (
	"bytes"
	"fmt"
	"image"

	"github.com/karty-game/karty-webgpu/internal/modeldata"
	"github.com/karty-game/karty-webgpu/pkg/assetbuild"
)

type textureKey struct {
	index int
	role  modeldata.TextureRole
}

func (build *compiler) prepareMaterials() error {
	for index, source := range build.document.Images {
		if source.URI != "" || source.BufferView == nil || *source.BufferView < 0 ||
			*source.BufferView >= len(build.document.BufferViews) ||
			source.MimeType != "image/png" && source.MimeType != "image/jpeg" {
			return fmt.Errorf("image %d must be embedded PNG/JPEG: %w", index, errUnsupported)
		}
	}

	for index, source := range build.document.Samplers {
		if source.WrapS != 0 && source.WrapS != 10497 || source.WrapT != 0 && source.WrapT != 10497 ||
			source.MagFilter != 0 && source.MagFilter != 9729 || source.MinFilter != 0 && source.MinFilter != 9987 {
			return fmt.Errorf("sampler %d requires repeat and linear mip filtering: %w", index, errUnsupported)
		}
	}

	build.materials = make([]modeldata.Material, len(build.document.Materials))
	for index, source := range build.document.Materials {
		value, err := build.prepareMaterial(source)
		if err != nil {
			return fmt.Errorf("material %d (%s): %w", index, source.Name, err)
		}

		build.materials[index] = value
	}

	return nil
}

func (build *compiler) prepareMaterial(source gltfMaterial) (modeldata.Material, error) {
	value := defaultMaterial()

	value.DoubleSided = source.DoubleSided
	if source.AlphaMode != "" && source.AlphaMode != "OPAQUE" && source.AlphaMode != "MASK" || source.EmissiveTexture != nil ||
		source.PBRMetallicRoughness.MetallicRoughnessTexture != nil || source.PBRMetallicRoughness.MetallicFactor == nil ||
		*source.PBRMetallicRoughness.MetallicFactor != 0 {
		return value, fmt.Errorf("alpha, emissive or metallic workflow (metallicFactor must be explicit zero): %w", errUnsupported)
	}

	for _, emissive := range source.EmissiveFactor {
		if emissive != 0 {
			return value, fmt.Errorf("emissive factor: %w", errUnsupported)
		}
	}

	value, err := materialCoverage(value, source)
	if err != nil {
		return value, err
	}

	value, err = materialFactors(value, source.PBRMetallicRoughness)
	if err != nil {
		return value, err
	}

	pbr := source.PBRMetallicRoughness
	if pbr.BaseColorTexture != nil {
		value.Albedo, err = build.texture(pbr.BaseColorTexture.Index, pbr.BaseColorTexture.TexCoord, modeldata.Albedo)
		if err != nil {
			return value, err
		}
	}

	if source.NormalTexture != nil {
		value.Normal, err = build.texture(source.NormalTexture.Index, source.NormalTexture.TexCoord, modeldata.Normal)
		if err != nil {
			return value, err
		}

		if source.NormalTexture.Scale != nil {
			value.NormalStrength = *source.NormalTexture.Scale
		}
	}

	return build.occlusion(value, source.OcclusionTexture)
}

func (build *compiler) occlusion(value modeldata.Material, source *occlusionTexture) (modeldata.Material, error) {
	if source == nil {
		return value, nil
	}

	index, err := build.texture(source.Index, source.TexCoord, modeldata.Occlusion)
	if err != nil {
		return value, err
	}

	value.AO = index
	if source.Strength != nil {
		value.AOStrength = *source.Strength
	}

	return value, nil
}

func defaultMaterial() modeldata.Material {
	return modeldata.Material{Color: [4]float32{1, 1, 1, 1}, Roughness: 1, NormalStrength: 1, AOStrength: 1, Albedo: -1, Normal: -1, AO: -1}
}

func (build *compiler) texture(index, coordinates int, role modeldata.TextureRole) (int32, error) {
	if coordinates != 0 || index < 0 || index >= len(build.document.Textures) {
		return 0, fmt.Errorf("texture index or UV set (only TEXCOORD_0 supported): %w", errUnsupported)
	}

	source := build.document.Textures[index]
	if source.Source == nil || *source.Source < 0 || *source.Source >= len(build.document.Images) ||
		source.Sampler != nil && (*source.Sampler < 0 || *source.Sampler >= len(build.document.Samplers)) {
		return 0, errFormat
	}

	key := textureKey{*source.Source, role}
	if previous, ok := build.textureCache[key]; ok {
		return previous, nil
	}

	if len(build.output.Textures) >= modeldata.MaxTextures {
		return 0, errBounds
	}

	data, err := build.image(*source.Source, role)
	if err != nil {
		return 0, err
	}

	//nolint:gosec // The prepared texture count is bounded to 64 before conversion.
	prepared := int32(len(build.output.Textures))
	build.output.Textures = append(build.output.Textures, modeldata.Texture{Role: role, Data: data})
	build.textureBytes += len(data)
	build.textureCache[key] = prepared

	return prepared, nil
}

func (build *compiler) image(index int, role modeldata.TextureRole) ([]byte, error) {
	source := build.document.Images[index]
	view := build.document.BufferViews[*source.BufferView]
	encoded := build.binary[view.ByteOffset : view.ByteOffset+view.ByteLength]

	config, format, err := image.DecodeConfig(bytes.NewReader(encoded))
	if err != nil || format != "png" && source.MimeType == "image/png" || format != "jpeg" && source.MimeType == "image/jpeg" {
		return nil, fmt.Errorf("image %d MIME/content mismatch: %w", index, errFormat)
	}

	estimate, err := textureSize(config.Width, config.Height)
	if err != nil || estimate > modeldata.MaxBytes-build.textureBytes {
		return nil, errBounds
	}

	usage := []string{assetbuild.UsageAlbedo, assetbuild.UsageNormal, assetbuild.UsageAO}[role]

	value, err := assetbuild.DecodeImage(encoded, fmt.Sprintf("texture%d", index), build.output.Source, usage)
	if err != nil {
		return nil, err
	}

	if build.options.MaxTextureDimension > 0 {
		value, err = assetbuild.ResizeImage(value, assetbuild.ResizeOptions{MaxDimension: build.options.MaxTextureDimension})
		if err != nil {
			return nil, err
		}
	}

	mips, err := assetbuild.GenerateMipmaps(value)
	if err != nil {
		return nil, err
	}

	return assetbuild.EncodeTexture(mips)
}

func textureSize(width, height int) (int, error) {
	const (
		maxDimension                            = 4096
		ktxHeaderAndDescriptor, levelIndexBytes = 172, 24
	)

	if width < 1 || height < 1 || width > maxDimension || height > maxDimension {
		return 0, errBounds
	}

	size := ktxHeaderAndDescriptor
	for {
		size += width*height*wordSize + levelIndexBytes
		if width == 1 && height == 1 {
			return size, nil
		}

		width, height = max(1, width/2), max(1, height/2)
	}
}

func materialFactors(value modeldata.Material, pbr pbr) (modeldata.Material, error) {
	if pbr.BaseColorFactor != nil {
		if len(pbr.BaseColorFactor) != len(value.Color) {
			return value, errFormat
		}

		for _, channel := range pbr.BaseColorFactor {
			if !finite(float64(channel)) || channel < 0 || channel > 1 {
				return value, errFormat
			}
		}

		copy(value.Color[:], pbr.BaseColorFactor)
	}

	// OPAQUE ignores authored coverage alpha in glTF; make that semantic final offline.
	if !value.AlphaMask {
		value.Color[3] = 1
	}

	if pbr.RoughnessFactor != nil {
		value.Roughness = *pbr.RoughnessFactor
	}

	return value, nil
}

func materialCoverage(value modeldata.Material, source gltfMaterial) (modeldata.Material, error) {
	if source.AlphaMode != "MASK" {
		return value, nil
	}

	const defaultMaskCutoff = .5

	value.AlphaMask = true

	value.AlphaCutoff = defaultMaskCutoff
	if source.AlphaCutoff != nil {
		value.AlphaCutoff = *source.AlphaCutoff
	}

	if !finite(float64(value.AlphaCutoff)) || value.AlphaCutoff < 0 || value.AlphaCutoff > 1 {
		return value, errFormat
	}

	return value, nil
}
