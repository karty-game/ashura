//go:build !js

package main

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"sync/atomic"
)

// Keep project-level concurrency bounded: recipes may hold large model bundles.
// Complete all recipes before loading imports for the shared browser build.
func prepareWebAssets(ctx context.Context, root string, packages []webPackage, options assetOptions) error {
	const maxWorkers = 4

	ctx, cancel := context.WithCancelCause(ctx)
	defer cancel(nil)

	if options.log == nil {
		options.log = os.Stdout
	}

	var (
		workers sync.WaitGroup
		logs    sync.Mutex
		next    atomic.Int64
	)

	for range min(len(packages), runtime.GOMAXPROCS(0), maxWorkers) {
		workers.Go(func() {
			for ctx.Err() == nil {
				index := int(next.Add(1) - 1)
				if index >= len(packages) {
					return
				}

				item := packages[index]

				var output bytes.Buffer

				local := options
				local.log = &output
				err := prepareProjectAssets(ctx, filepath.Join(root, item.RelativePath), local)

				logs.Lock()
				if output.Len() > 0 {
					_, _ = fmt.Fprintf(options.log, "Assets %s:\n%s", item.ImportPath, &output)
				}
				logs.Unlock()

				if err != nil {
					cancel(fmt.Errorf("prepare %s: %w", item.ImportPath, err))

					return
				}
			}
		})
	}

	workers.Wait()

	return context.Cause(ctx)
}

// One go build loads the dependency graph once and schedules independent mains
// concurrently using Go's own -p limit. Duplicate executable names use separate
// batches so packages in different directories cannot overwrite each other.
func buildWebPackages(ctx context.Context, root, dist string, packages []webPackage, compiler webCompiler) error {
	// TinyGo accepts one main package per invocation. Keep its LLVM linkers
	// sequential to bound memory; standard Go retains its shared graph batching.
	if compiler.Name == tinyGoCompiler {
		for _, item := range packages {
			destination := filepath.Join(dist, "apps", item.RelativePath)
			_, _ = fmt.Fprintf(os.Stdout, "Building %s for browser with TinyGo...\n", item.ImportPath)

			if err := compiler.build(ctx, root, destination, item.ImportPath); err != nil {
				return err
			}
		}

		return nil
	}

	staging, err := os.MkdirTemp(dist, ".wasm-build-")
	if err != nil {
		return fmt.Errorf("stage browser builds: %w", err)
	}
	//nolint:gosec // staging is exclusively the fresh directory returned by MkdirTemp above.
	defer func() { _ = os.RemoveAll(staging) }()

	for _, batch := range webBuildBatches(packages) {
		arguments := []string{"build", "-o", staging + string(filepath.Separator)}
		for _, item := range batch {
			arguments = append(arguments, item.ImportPath)
			_, _ = fmt.Fprintf(os.Stdout, "Building %s for browser...\n", item.ImportPath)
		}

		//nolint:gosec // Package paths are separate argv values, never shell commands.
		command := exec.CommandContext(ctx, compiler.executable, arguments...)
		command.Dir = root

		command.Env = webBuildEnvironment(compiler.Name)
		command.Stdout, command.Stderr = os.Stdout, os.Stderr

		if err := command.Run(); err != nil {
			return fmt.Errorf("build browser packages: %w", err)
		}

		for _, item := range batch {
			destination := filepath.Join(dist, "apps", item.RelativePath, wasmFile)
			//nolint:gosec // Package paths are confined by localWebPackage; both directories are fresh staging trees.
			if err := os.Rename(filepath.Join(staging, item.BinaryName), destination); err != nil {
				return fmt.Errorf("stage browser package %s: %w", item.ImportPath, err)
			}
		}
	}

	return nil
}

func webBuildBatches(packages []webPackage) [][]webPackage {
	var batches [][]webPackage

	counts := make(map[string]int)
	for _, item := range packages {
		index := counts[item.BinaryName]
		if index == len(batches) {
			batches = append(batches, nil)
		}

		batches[index] = append(batches[index], item)
		counts[item.BinaryName]++
	}

	return batches
}
