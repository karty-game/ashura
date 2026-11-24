//go:build !js

package main

import (
	"context"
	"fmt"
	"html"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/karty-game/ashura/internal/platform/browser/framehost"
)

// buildWebPreview stages one complete immutable distribution before it is served.
// A single package retains the original full-window page; multiple main packages
// get individual pages and one gallery sharing the Go runtime script.
func buildWebPreview(ctx context.Context, root, dist, pattern string, options webOptions) error {
	packages, err := listWebPackages(ctx, root, pattern)
	if err != nil {
		return err
	}

	return stageWebDistribution(ctx, root, dist, pattern, packages, options)
}

func stageWebDistribution(ctx context.Context, root, dist, pattern string, packages []webPackage, options webOptions) error {
	groups, err := planWebCompilers(ctx, packages, options.compiler)
	if err != nil {
		return err
	}

	if err := prepareWebAssets(ctx, root, packages, options.assets); err != nil {
		return err
	}

	if len(groups) > 1 {
		return stageMixedWebDistribution(ctx, root, dist, packages, groups)
	}

	compiler := groups[0].compiler

	if err := compiler.stageRuntime(dist); err != nil {
		return err
	}

	if len(packages) == 1 {
		if err := stageWebResources(filepath.Join(root, packages[0].RelativePath), dist); err != nil {
			return err
		}

		if err := compiler.build(ctx, root, dist, buildPackagePath(packages[0], pattern)); err != nil {
			return err
		}

		return writeSamplePage(dist, "./"+wasmExecFile, packages[0].Name)
	}

	samples := make([]gallerySample, 0, len(packages))
	for _, item := range packages {
		path := filepath.Join("apps", item.RelativePath)
		if _, err := stageWebApplication(root, filepath.Join(dist, path), filepath.Join(dist, wasmExecFile), item); err != nil {
			return err
		}

		samples = append(samples, gallerySample{Name: item.Name, Path: relativeWebPath(path), Description: item.Description})
	}

	if err := buildWebPackages(ctx, root, dist, packages, compiler); err != nil {
		return err
	}

	return writeGallery(dist, samples)
}

func stageWebApplication(root, destination, runtimeFile string, item webPackage) (string, error) {
	//nolint:gosec // The destination is inside fresh staging; localWebPackage confines item.RelativePath.
	if err := os.MkdirAll(destination, 0o750); err != nil {
		return "", fmt.Errorf("stage application: %w", err)
	}

	if err := stageWebResources(filepath.Join(root, item.RelativePath), destination); err != nil {
		return "", err
	}

	runtimePath, err := filepath.Rel(destination, runtimeFile)
	if err != nil {
		return "", fmt.Errorf("resolve compiler runtime: %w", err)
	}

	runtimePath = filepath.ToSlash(runtimePath)

	return runtimePath, writeSamplePage(destination, runtimePath, item.Name)
}

func buildPackagePath(item webPackage, pattern string) string {
	if item.ImportPath == "command-line-arguments" {
		return pattern
	}

	return item.ImportPath
}

func relativeWebPath(path string) string {
	parts := strings.Split(filepath.ToSlash(path), "/")
	for index, part := range parts {
		parts[index] = url.PathEscape(part)
	}

	return strings.Join(parts, "/") + "/"
}

// writeSamplePage keeps both runtime and WASM URLs relative to the sample page.
func writeSamplePage(dist, runtimePath, title string) error {
	page := strings.ReplaceAll(indexHTML, "__ASHURA_FRAME_HOST__", framehost.Source)
	page = strings.ReplaceAll(page, "__WASM_RUNTIME__", html.EscapeString(runtimePath))
	page = strings.ReplaceAll(page, "__SAMPLE_TITLE__", html.EscapeString(title+" — Ashura"))
	//nolint:gosec // dist is a fresh preview/export staging directory; the filename is fixed.
	if err := os.WriteFile(filepath.Join(dist, "index.html"), []byte(page), 0o600); err != nil {
		return fmt.Errorf("write browser page: %w", err)
	}

	return nil
}

const indexHTML = `<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width,initial-scale=1">
  <title>__SAMPLE_TITLE__</title>
  <style>
    html, body { width: 100%; height: 100%; margin: 0; overflow: hidden; background: #17191f; }
    canvas { display: block; width: 100%; height: 100%; }
    #error { position: fixed; inset: 1rem 1rem auto; color: #ffd2d2; font: 14px monospace; white-space: pre-wrap; }
  </style>
</head>
<body>
  <canvas id="game"></canvas>
  <pre id="error"></pre>
  <script src="__WASM_RUNTIME__"></script>
  <script>
__ASHURA_FRAME_HOST__
    const go = new Go();
    WebAssembly.instantiateStreaming(fetch("./app.wasm"), go.importObject)
      .then(({ instance }) => go.run(instance))
      .catch((error) => { document.getElementById("error").textContent = error.stack || String(error); });
  </script>
</body>
</html>
`
