//go:build !js

package levelbuild

import "github.com/karty-game/ashura/internal/leveldata"

func pagePublicationSchema(schema string, page leveldata.Page) string {
	if schema == leveldata.Schema7 || schema == leveldata.Schema8 {
		return schema
	}

	if page.Representation != "" {
		return leveldata.Schema6
	}

	if schema == leveldata.Schema && (page.Width > 1024 || page.Height > 1024) {
		return leveldata.Schema2
	}

	return schema
}
