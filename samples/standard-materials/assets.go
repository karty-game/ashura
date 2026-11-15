package main

import (
	"github.com/karty-game/karty-webgpu/pkg/engine"
	"github.com/karty-game/karty-webgpu/pkg/material"
	"github.com/karty-game/karty-webgpu/pkg/texture/ktx2"
	"github.com/karty-game/karty-webgpu/samples/standard-materials/generated/assets"
)

type materialPair struct {
	standard material.Material
	source   material.SourceMaterial
}

func (pair *materialPair) setUVScale(scale engine.Vector2) {
	pair.standard.UVScale, pair.source.UVScale = scale, scale
}

func loadMaterials(app *engine.App) (materialPair, materialPair, error) {
	stone, err := loadMaterialPair(app, assets.StoneAlbedo(), assets.StoneNormal(), assets.StoneAO(), assets.StoneSSBump(), stoneRoughness)
	if err != nil {
		return materialPair{}, materialPair{}, err
	}

	brick, err := loadMaterialPair(app, assets.BrickAlbedo(), assets.BrickNormal(), assets.BrickAO(), assets.BrickSSBump(), brickRoughness)

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

	standard := material.DefaultStandard()
	standard.AlbedoTexture, standard.NormalTexture, standard.AOTexture = albedo, normal, occlusion
	standard.Roughness = roughness
	source := material.DefaultSource()
	source.AlbedoTexture, source.SSBumpTexture, source.AmbientColors = albedo, ssbump, ambientColors()

	return materialPair{standard: standard, source: source}, nil
}
