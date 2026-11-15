//go:build !js

package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/karty-game/karty-webgpu/pkg/assetbuild"
)

const (
	assetCommand       = "assets"
	buildCommand       = "build"
	defaultAddress     = "127.0.0.1:0"
	defaultPackage     = "./samples/basic-scene"
	wasmFile           = "app.wasm"
	wasmExecFile       = "wasm_exec.js"
	previewTimeout     = 5 * time.Second
	previewReadTimeout = 5 * time.Second
)

var (
	errUsage = errors.New(
		"usage: engine run web [go-package-or-pattern] [--addr host:port] [--compiler go|tinygo] [--profile dev] [--no-cache] | " +
			"engine build web [go-package-or-pattern] [--output dist/samples] [--compiler go|tinygo] [--profile dev] [--no-cache] | " +
			"engine assets build [--project directory | --source assets --output generated/assets] [--jobs count] | " +
			"engine assets generate [go-package-or-pattern | --project directory] [--profile dev] | " +
			"engine assets model --input source.glb --output prepared.kmodel | " +
			"engine assets texture --input source.png --output prepared.ktx2 --usage albedo|normal|ao|ssbump",
	)
	errMissingAddress      = errors.New("--addr requires a host:port value")
	errModuleRootNotFound  = errors.New("could not find go.mod; run this command inside the engine module")
	errWasmRuntimeNotFound = errors.New("could not find wasm_exec.js under the selected compiler root")
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(args []string) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if len(args) >= 2 && args[0] == buildCommand && args[1] == "web" {
		return exportWeb(ctx, args[2:])
	}

	if len(args) >= 2 && args[0] == assetCommand {
		switch args[1] {
		case "generate":
			return runAssetCatalog(ctx, args[2:], assetbuild.PhaseGenerate)
		case "model":
			return buildModel(args[2:])
		case "lightmap":
			return buildLightmap(ctx, args[2:])
		case buildCommand:
			return buildAssetCatalog(ctx, args[2:])
		case "texture":
			return buildTexture(args[2:])
		default:
			return errUsage
		}
	}

	if len(args) < 2 || args[0] != "run" || args[1] != "web" {
		return errUsage
	}

	previewArgs, options, err := parseWebOptions(args[2:])
	if err != nil {
		return err
	}

	packagePath, address, err := parseRunArgs(previewArgs)
	if err != nil {
		return err
	}

	root, err := moduleRoot()
	if err != nil {
		return err
	}

	dist, err := os.MkdirTemp("", "karty-webgpu-web-")
	if err != nil {
		return fmt.Errorf("create temporary web build directory: %w", err)
	}
	defer func() {
		_ = os.RemoveAll(dist)
	}()

	if err := buildWebPreview(ctx, root, dist, packagePath, options); err != nil {
		return err
	}

	return serve(ctx, dist, address, packagePath)
}

func parseRunArgs(args []string) (string, string, error) {
	packagePath := defaultPackage
	address := defaultAddress
	hasPackage := false

	for index := 0; index < len(args); index++ {
		argument := args[index]
		switch {
		case argument == "--addr":
			if index+1 >= len(args) {
				return "", "", errMissingAddress
			}

			index++

			address = args[index]
			if address == "" {
				return "", "", errMissingAddress
			}
		case strings.HasPrefix(argument, "--addr="):
			address = strings.TrimPrefix(argument, "--addr=")
			if address == "" {
				return "", "", errMissingAddress
			}
		default:
			if strings.HasPrefix(argument, "-") || hasPackage {
				return "", "", errUsage
			}

			packagePath = argument
			hasPackage = true
		}
	}

	return packagePath, address, nil
}

func moduleRoot() (string, error) {
	current, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("read current directory: %w", err)
	}

	for {
		if _, err := os.Stat(filepath.Join(current, "go.mod")); err == nil {
			return current, nil
		}

		parent := filepath.Dir(current)
		if parent == current {
			return "", errModuleRootNotFound
		}

		current = parent
	}
}

func serve(ctx context.Context, dist, address, packagePath string) error {
	listener, err := (&net.ListenConfig{}).Listen(ctx, "tcp", address)
	if err != nil {
		return fmt.Errorf("listen on %s: %w", address, err)
	}

	server := &http.Server{Handler: http.FileServer(http.Dir(dist)), ReadHeaderTimeout: previewReadTimeout}

	serveErrors := make(chan error, 1)
	go func() {
		serveErrors <- server.Serve(listener)
	}()

	_, _ = fmt.Fprintf(os.Stdout, "Serving %s at http://%s\nPress Ctrl+C to stop.\n", packagePath, listener.Addr())

	select {
	case err := <-serveErrors:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}

		return fmt.Errorf("serve browser build: %w", err)
	case <-ctx.Done():
		shutdownContext, cancel := context.WithTimeout(context.WithoutCancel(ctx), previewTimeout)
		defer cancel()

		if err := server.Shutdown(shutdownContext); err != nil {
			return fmt.Errorf("stop preview server: %w", err)
		}

		return nil
	}
}
