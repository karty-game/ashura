//go:build !js

package assetbuild

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
)

// DefaultConfig selects the standard PNG/JPEG, mipmap, KTX2 and Go catalog recipe.
func DefaultConfig(input []string, output string) Config {
	return Config{Output: output, Pipelines: []Pipeline{{
		Name: "textures", Input: input,
		Import: []Plugin{Images()}, Process: []Plugin{Mipmaps()}, Output: []Plugin{KTX2(), GoCatalog()},
	}}}
}

// BuildDirectory preserves explicit source/output directory workflows. Paths are
// relative to the caller, and the nearest common ancestor owns this invocation.
func BuildDirectory(ctx context.Context, source, output string, options Options) (Report, error) {
	source, err := filepath.Abs(source)
	if err != nil {
		return Report{}, fmt.Errorf("source directory: %w", err)
	}

	output, err = filepath.Abs(output)
	if err != nil {
		return Report{}, fmt.Errorf("output directory: %w", err)
	}

	if overlaps(source, output) {
		return Report{}, fmt.Errorf("source and output directories overlap: %w", errPath)
	}

	info, err := os.Stat(source)
	if err != nil {
		return Report{}, fmt.Errorf("source directory: %w", err)
	}

	if !info.IsDir() {
		return Report{}, fmt.Errorf("source must be a directory: %w", errPath)
	}

	project := filepath.Dir(source)
	for {
		relative, relErr := filepath.Rel(project, output)
		if relErr == nil && filepath.IsLocal(relative) {
			break
		}

		parent := filepath.Dir(project)
		if parent == project {
			return Report{}, fmt.Errorf("source and output have no common owner: %w", errPath)
		}

		project = parent
	}

	sourceRelative, err := filepath.Rel(project, source)
	if err != nil {
		return Report{}, fmt.Errorf("source identity: %w", err)
	}

	outputRelative, err := filepath.Rel(project, output)
	if err != nil {
		return Report{}, fmt.Errorf("output identity: %w", err)
	}

	options.Project = project

	return Build(ctx, DefaultConfig([]string{filepath.ToSlash(sourceRelative) + "/**"}, filepath.ToSlash(outputRelative)), options)
}

// TextureInfo describes one explicit prepared texture conversion.
type TextureInfo struct{ Width, Height, MipLevels, Bytes int }

// PrepareTexture converts an explicit source image using the same role-aware
// import, mip and KTX2 algorithms as the default plugin recipe.
func PrepareTexture(input, output, usage string) (TextureInfo, error) {
	input, err := filepath.Abs(input)
	if err != nil {
		return TextureInfo{}, fmt.Errorf("texture source: %w", err)
	}

	output, err = filepath.Abs(output)
	if err != nil {
		return TextureInfo{}, fmt.Errorf("texture output: %w", err)
	}

	if input == output || !validUsage(usage) {
		return TextureInfo{}, errSourceImage
	}

	inputRoot, err := projectRoot(filepath.Dir(input))
	if err != nil {
		return TextureInfo{}, err
	}

	encoded, err := readProjectFile(inputRoot, filepath.Base(input))
	if err != nil {
		return TextureInfo{}, err
	}

	value, err := DecodeImage(encoded, "Texture", filepath.Base(input), usage)
	if err != nil {
		return TextureInfo{}, err
	}

	value, err = GenerateMipmaps(value)
	if err != nil {
		return TextureInfo{}, err
	}

	prepared, err := EncodeTexture(value)
	if err != nil {
		return TextureInfo{}, err
	}

	outputRoot, err := projectRoot(filepath.Dir(output))
	if err != nil {
		return TextureInfo{}, err
	}

	output, err = confined(outputRoot, filepath.Base(output))
	if err != nil {
		return TextureInfo{}, err
	}

	if _, err := writeUnchanged(output, prepared); err != nil {
		return TextureInfo{}, err
	}

	return TextureInfo{Width: value.Width, Height: value.Height, MipLevels: len(value.Levels), Bytes: len(prepared)}, nil
}
