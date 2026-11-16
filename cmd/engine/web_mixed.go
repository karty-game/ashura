//go:build !js

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

type webCompilerGroup struct {
	compiler webCompiler
	packages []webPackage
}

type webApplicationReceipt struct {
	webCompiler

	Path        string `json:"path"`
	RuntimePath string `json:"runtimePath"`
}

type webDistributionReceipt struct {
	FormatVersion int                     `json:"formatVersion"`
	Compiler      string                  `json:"compiler"`
	Applications  []webApplicationReceipt `json:"applications"`
}

func sampleWebCompiler(item webPackage, requested string) string {
	if requested != "" && requested != automaticWebCompiler {
		return requested
	}

	// Match both module and relative path; copies and unrelated projects use Go.
	if name, _ := repositorySampleBudget(item); name == "basic-scene" {
		return tinyGoCompiler
	}

	return "go"
}

func planWebCompilers(ctx context.Context, packages []webPackage, requested string) ([]webCompilerGroup, error) {
	if len(packages) == 0 {
		return nil, errNoWebPackages
	}

	var groups []webCompilerGroup

	indices := make(map[string]int)

	for _, item := range packages {
		name := sampleWebCompiler(item, requested)

		index, exists := indices[name]
		if !exists {
			compiler, err := resolveWebCompiler(ctx, name)
			if err != nil {
				return nil, err
			}

			index = len(groups)
			indices[name] = index

			groups = append(groups, webCompilerGroup{compiler: compiler})
		}

		groups[index].packages = append(groups[index].packages, item)
	}

	return groups, nil
}

func stageMixedWebDistribution(ctx context.Context, root, dist string, packages []webPackage, groups []webCompilerGroup) error {
	compilers := make(map[string]webCompiler)

	for _, group := range groups {
		runtimeDir := filepath.Join(dist, "runtimes", group.compiler.Name)
		//nolint:gosec // Compiler names are admitted go/tinygo constants in fresh staging.
		if err := os.MkdirAll(runtimeDir, 0o750); err != nil {
			return fmt.Errorf("stage compiler runtime: %w", err)
		}

		if err := group.compiler.stageRuntime(runtimeDir); err != nil {
			return err
		}

		compilers[group.compiler.Name] = group.compiler
	}

	receipt := webDistributionReceipt{FormatVersion: 1, Compiler: "mixed"}

	samples := make([]gallerySample, 0, len(packages))
	for _, item := range packages {
		compiler := compilers[sampleWebCompiler(item, automaticWebCompiler)]

		app, err := stageMixedWebApplication(root, dist, item, compiler)
		if err != nil {
			return err
		}

		receipt.Applications = append(receipt.Applications, app)
		samples = append(samples, gallerySample{Name: item.Name, Path: relativeWebPath(app.Path)})
	}

	for _, group := range groups {
		if err := buildWebPackages(ctx, root, dist, group.packages, group.compiler); err != nil {
			return err
		}
	}

	if err := writeMixedWebReceipt(dist, receipt); err != nil {
		return err
	}

	return writeGallery(dist, samples)
}

func stageMixedWebApplication(root, dist string, item webPackage, compiler webCompiler) (webApplicationReceipt, error) {
	path := filepath.Join("apps", item.RelativePath)
	destination := filepath.Join(dist, path)
	//nolint:gosec // item.RelativePath is admitted by localWebPackage inside fresh staging.
	if err := os.MkdirAll(destination, 0o750); err != nil {
		return webApplicationReceipt{}, fmt.Errorf("stage application: %w", err)
	}

	if err := stageWebResources(filepath.Join(root, item.RelativePath), destination); err != nil {
		return webApplicationReceipt{}, err
	}

	runtimePath, err := filepath.Rel(destination, filepath.Join(dist, "runtimes", compiler.Name, wasmExecFile))
	if err != nil {
		return webApplicationReceipt{}, fmt.Errorf("resolve compiler runtime: %w", err)
	}

	if err := writeSamplePage(destination, filepath.ToSlash(runtimePath), item.Name); err != nil {
		return webApplicationReceipt{}, err
	}

	app := webApplicationReceipt{Path: filepath.ToSlash(path), RuntimePath: filepath.ToSlash(runtimePath), webCompiler: compiler}
	if err := writeMixedWebReceipt(destination, app); err != nil {
		return webApplicationReceipt{}, err
	}

	return app, nil
}

func writeMixedWebReceipt(dist string, receipt any) error {
	data, err := json.MarshalIndent(receipt, "", "  ")
	if err != nil {
		return fmt.Errorf("encode compiler receipt: %w", err)
	}

	//nolint:gosec // Fixed receipt name under fresh private distribution staging.
	if err := os.WriteFile(filepath.Join(dist, "build-info.json"), append(data, '\n'), 0o600); err != nil {
		return fmt.Errorf("write compiler receipt: %w", err)
	}

	return nil
}
