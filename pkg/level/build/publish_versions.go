//go:build !js

package levelbuild

import "github.com/karty-game/karty-webgpu/internal/leveldata"

func pagePublicationSchema(schema string, page leveldata.Page) string {
	if schema == leveldata.Schema7 {
		return leveldata.Schema7
	}

	if page.Representation != "" {
		return leveldata.Schema6
	}

	if schema == leveldata.Schema && (page.Width > 1024 || page.Height > 1024) {
		return leveldata.Schema2
	}

	return schema
}
