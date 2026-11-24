package main

import (
	"errors"

	"github.com/karty-game/ashura/pkg/engine"
	"github.com/karty-game/ashura/pkg/material"
	"github.com/karty-game/ashura/pkg/texture"
	"github.com/karty-game/ashura/pkg/texture/ktx2"
	"github.com/karty-game/ashura/samples/standard-materials/generated/assets"
)

var errMaterialCompression = errors.New("materials requires BC texture compression or both ETC2 and ASTC")

type materialPair struct {
	standard material.Material
	source   material.SourceMaterial
}

func (pair *materialPair) setUVScale(scale engine.Vector2) {
	pair.standard.UVScale, pair.source.UVScale = scale, scale
}

func materialFormat(supports func(texture.Format) bool) (texture.Format, error) {
	if supports(texture.BC1) && supports(texture.BC7) {
		return texture.BC1, nil
	}

	if supports(texture.ETC2RGB) && supports(texture.ETC2RGBA) && supports(texture.ASTC4x4) {
		return texture.ETC2RGB, nil
	}

	return texture.RGBA8, errMaterialCompression
}

func loadMaterialsForFormat(app *engine.App, format texture.Format) (materialPair, materialPair, error) {
	var sources [2][4]ktx2.Asset

	switch format {
	case texture.BC1:
		sources = [2][4]ktx2.Asset{
			{assets.StoneAlbedo(), assets.StoneNormal(), assets.StoneAO(), assets.StoneSSBump()},
			{assets.BrickAlbedo(), assets.BrickNormal(), assets.BrickAO(), assets.BrickSSBump()},
		}
	case texture.ETC2RGB:
		sources = [2][4]ktx2.Asset{
			{assets.StoneAlbedoMobile(), assets.StoneNormalMobile(), assets.StoneAO(), assets.StoneSSBumpMobile()},
			{assets.BrickAlbedoMobile(), assets.BrickNormalMobile(), assets.BrickAO(), assets.BrickSSBumpMobile()},
		}
	case texture.RGBA8, texture.BC6H, texture.BC7, texture.ETC2RGBA, texture.ASTC4x4, texture.ASTC6x6:
		return materialPair{}, materialPair{}, errMaterialCompression
	default:
		return materialPair{}, materialPair{}, errMaterialCompression
	}

	stone, err := loadMaterialPair(app, sources[0][0], sources[0][1], sources[0][2], sources[0][3], stoneRoughness)
	if err != nil {
		return materialPair{}, materialPair{}, err
	}

	brick, err := loadMaterialPair(app, sources[1][0], sources[1][1], sources[1][2], sources[1][3], brickRoughness)

	return stone, brick, err
}

func loadMaterialPair(app *engine.App, albedoAsset, normalAsset, aoAsset, ssbumpAsset ktx2.Asset, roughness float32) (materialPair, error) {
	albedo, err := albedoAsset.Load(app)
	if err != nil {
		return materialPair{}, err
	}

	normal, err := normalAsset.Load(app)
	if err != nil {
		return materialPair{}, err
	}

	occlusion, err := aoAsset.Load(app)
	if err != nil {
		return materialPair{}, err
	}

	ssbump, err := ssbumpAsset.Load(app)
	if err != nil {
		return materialPair{}, err
	}

	pair := defaultMaterialPair(roughness)
	pair.standard.AlbedoTexture, pair.standard.NormalTexture, pair.standard.AOTexture = albedo, normal, occlusion
	pair.source.AlbedoTexture, pair.source.SSBumpTexture = albedo, ssbump

	return pair, nil
}

func defaultMaterialPair(roughness float32) materialPair {
	standard := material.DefaultStandard()
	standard.Roughness = roughness
	source := material.DefaultSource()
	source.AmbientColors = ambientColors()

	return materialPair{standard: standard, source: source}
}
