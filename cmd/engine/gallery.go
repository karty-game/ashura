//go:build !js

package main

import (
	"bytes"
	_ "embed"
	"errors"
	"fmt"
	"html/template"
	"os"
	"path/filepath"
)

var errEmptyGallery = errors.New("sample gallery has no samples")

//go:embed gallery.html
var galleryHTML string

type gallerySample struct {
	Name string
	Path string
}

func writeGallery(dist string, samples []gallerySample) error {
	if len(samples) == 0 {
		return errEmptyGallery
	}

	galleryPage, err := template.New("gallery").Parse(galleryHTML)
	if err != nil {
		return fmt.Errorf("parse sample gallery page: %w", err)
	}

	var page bytes.Buffer

	if err := galleryPage.Execute(&page, samples); err != nil {
		return fmt.Errorf("render sample gallery: %w", err)
	}

	//nolint:gosec // dist is a fresh preview/export staging directory; the filename is fixed.
	if err := os.WriteFile(filepath.Join(dist, "index.html"), page.Bytes(), 0o600); err != nil {
		return fmt.Errorf("write sample gallery: %w", err)
	}

	return nil
}
