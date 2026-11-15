//go:build !js

package main

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

const (
	defaultSampleLimit   int64 = 20_000_000
	impostorsSampleLimit int64 = 30_000_000
	sponzaSampleLimit    int64 = 35_000_000
	repositorySamples          = "github.com/karty-game/karty-webgpu/samples/"
)

var errBudget = errors.New("sample export exceeds its complete-file budget")
var errLayout = errors.New("invalid sample gallery layout")

type sampleSize struct {
	name, path   string
	limit        int64
	wasm, assets int64
	hasWasm      bool
}
type gallerySize struct {
	samples       []sampleSize
	shared, total int64
	files         int
}

// Count logical lengths of every file. A sample includes its entire subtree;
// shared host/compiler files count against every sample as well as once in the site.
// No exclusions, sparse-filesystem accounting or compressed transfer sizes apply.
func inspectWebExport(root string, packages []webPackage) (gallerySize, error) {
	var report gallerySize

	for _, item := range packages {
		name, limit := repositorySampleBudget(item)
		if limit == 0 {
			continue
		}

		path := filepath.ToSlash(filepath.Join("apps", item.RelativePath))
		if len(packages) == 1 {
			path = "."
		}

		report.samples = append(report.samples, sampleSize{name: name, path: path, limit: limit})
	}

	//nolint:gosec // root is the export's private staging directory; traversal rejects every symlink before following any path.
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}

		return report.countFile(root, path, entry)
	})
	if err != nil {
		return gallerySize{}, err
	}

	for _, sample := range report.samples {
		if !sample.hasWasm {
			return gallerySize{}, fmt.Errorf("sample %s has no app.wasm: %w", sample.name, errLayout)
		}
	}

	return report, nil
}

func repositorySampleBudget(item webPackage) (string, int64) {
	name, local := strings.CutPrefix(item.ImportPath, repositorySamples)
	if !local || !fs.ValidPath(name) || name == "." || filepath.ToSlash(item.RelativePath) != "samples/"+name {
		return "", 0
	}

	if name == "sponza" {
		return name, sponzaSampleLimit
	}

	if name == "impostors" {
		return name, impostorsSampleLimit
	}

	return name, defaultSampleLimit
}

func checkWebSampleBudgets(root string, packages []webPackage, log io.Writer) error {
	report, err := inspectWebExport(root, packages)
	if err != nil {
		return err
	}

	if len(report.samples) == 0 {
		return nil
	}

	return printReport(log, report)
}

func (report *gallerySize) countFile(root, path string, entry fs.DirEntry) error {
	if entry.Type()&os.ModeSymlink != 0 {
		return fmt.Errorf("symlink %s: %w", path, errLayout)
	}

	if entry.IsDir() {
		return nil
	}

	info, err := entry.Info()
	if err != nil {
		return err
	}

	if !info.Mode().IsRegular() {
		return fmt.Errorf("nonregular file %s: %w", path, errLayout)
	}

	relative, err := filepath.Rel(root, path)
	if err != nil {
		return err
	}

	length := info.Size()
	report.total += length
	report.files++

	relative = filepath.ToSlash(relative)
	owned := false

	for index := range report.samples {
		sample := &report.samples[index]

		within, belongs := strings.CutPrefix(relative, sample.path+"/")
		if sample.path == "." {
			within, belongs = relative, true
		}

		if !belongs {
			continue
		}

		owned = true

		if within == wasmFile {
			sample.wasm += length
			sample.hasWasm = true
		} else {
			sample.assets += length
		}
	}

	if !owned {
		// Unrelated application subtrees count conservatively as shared bytes
		// against limited samples in mixed exports, along with orphan files.
		report.shared += length
	}

	return nil
}

func printReport(writer io.Writer, report gallerySize) error {
	if _, err := fmt.Fprintln(
		writer,
		"Sample bytes: WASM + every asset/host file + shared runtime; decimal limits, no compression allowance.",
	); err != nil {
		return err
	}

	var failures []error

	for _, sample := range report.samples {
		total := sample.wasm + sample.assets + report.shared
		if _, err := fmt.Fprintf(
			writer,
			"%-20s wasm=%d assets=%d shared=%d total=%d limit=%d\n",
			sample.name,
			sample.wasm,
			sample.assets,
			report.shared,
			total,
			sample.limit,
		); err != nil {
			return err
		}

		if total > sample.limit {
			failures = append(failures, fmt.Errorf("%s: %d > %d bytes: %w", sample.name, total, sample.limit, errBudget))
		}
	}

	if _, err := fmt.Fprintf(writer, "Entire gallery: %d logical bytes in %d files\n", report.total, report.files); err != nil {
		return err
	}

	return errors.Join(failures...)
}
