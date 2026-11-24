//go:build !js

package levelbuild

import (
	"github.com/karty-game/ashura/pkg/assetbuild"
	scenebuild "github.com/karty-game/ashura/pkg/scene/build"
)

const sceneSourceSchema = "karty.level-scene-source@1"

// SceneOptions selects authored scenes and their logical/model ownership.
// Material processing is configured separately with Materials.
type SceneOptions struct {
	Input          []string
	Modules        []scenebuild.Module
	Spaces         map[string]string
	SeparateMasked bool
}

type sceneSource struct {
	Scene          imported `json:"scene"`
	SeparateMasked bool     `json:"separateMasked"`
}

// Scenes imports authored scene structure and prepared model references without
// baking relief or requiring encoded image resources. Follow with Materials
// after image processing, then StaticChunks and the remaining level processors.
func Scenes(options SceneOptions) assetbuild.Plugin {
	return importPlugin{options: options}
}
