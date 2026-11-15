//go:build !js

package lightmapbuild

import (
	"bytes"
	"context"
	"crypto/rand"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/karty-game/karty-webgpu/internal/lightmapkernel"
	"github.com/karty-game/karty-webgpu/pkg/lightmap"
	"github.com/karty-game/karty-webgpu/pkg/texture"
)

//go:embed browser.js
var browserScript []byte

const (
	browserErrorRoute      = "error"
	browserScriptName      = "bake.js"
	browserHeaderTimeout   = 5 * time.Second
	browserTransferTimeout = 30 * time.Second
	browserHeaderBytes     = 16 << 10
)

var (
	errBrowserArguments = errors.New("browser lightmap bake requires context and asset")
	errBrowserExecution = errors.New("browser lightmap bake failed")
	errBrowserMissing   = errors.New("no Chromium found; supply BrowserOptions.Executable or --browser-executable")
)

// BrowserOptions names an installed Chromium with WebGPU support. No browser is
// downloaded or installed by the build. An empty name searches Chromium on PATH.
type BrowserOptions struct{ Executable string }

type browserExit struct{ err error }

type browserOutcome struct {
	pixels  []byte
	adapter string
	err     error
}

type browserLog struct{ bytes.Buffer }

func (log *browserLog) Write(data []byte) (int, error) {
	const maximum = 64 << 10
	if remaining := maximum - log.Len(); remaining > 0 {
		_, _ = log.Buffer.Write(data[:min(remaining, len(data))])
	}

	return len(data), nil
}

// BakeBrowser runs the same WGSL as lightmap.Baker on a hardware browser adapter.
// The owned listener is loopback-only; routes have a random capability token.
// Inputs are immutable and results bounded. All browser/profile/server resources
// are closed before returning; cancellation never publishes a partial result.
func BakeBrowser(ctx context.Context, asset *lightmap.Asset, point lightmap.Point, options BrowserOptions) (Result, error) {
	started := time.Now()

	if ctx == nil || asset == nil {
		return Result{}, errBrowserArguments
	}

	if err := ctx.Err(); err != nil {
		return Result{}, err
	}

	parameters, scale, err := lightmapkernel.Parameters(asset.Document(), point)
	if err != nil {
		return Result{}, err
	}

	executable, err := findBrowser(options.Executable)
	if err != nil {
		return Result{}, err
	}

	server, url, outcomes, err := startBakeServer(ctx, asset, parameters[:])
	if err != nil {
		return Result{}, err
	}
	defer func() { _ = server.Close() }()

	profile, err := os.MkdirTemp("", "karty-lightmap-browser-")
	if err != nil {
		return Result{}, err
	}

	defer func() { _ = os.RemoveAll(profile) }()

	command, logs, err := startBrowser(ctx, executable, profile, url)
	if err != nil {
		return Result{}, err
	}

	outcome, err := awaitBrowser(ctx, command, logs, outcomes)
	if err != nil {
		return Result{}, err
	}

	document := asset.Document()

	return Result{
		Texture: texture.Data{
			Width:      int(document.Width),
			Height:     int(document.Height),
			ColorSpace: texture.Linear,
			Levels:     [][]byte{outcome.pixels},
		},
		Scale:    scale,
		Adapter:  outcome.adapter,
		Duration: time.Since(started),
	}, nil
}

func awaitBrowser(ctx context.Context, command *exec.Cmd, logs *browserLog, outcomes <-chan browserOutcome) (browserOutcome, error) {
	finished := make(chan browserExit, 1)
	go func() { finished <- browserExit{err: command.Wait()} }()

	waited := false
	defer func() {
		if waited {
			return
		}

		_ = command.Process.Kill()

		select {
		case <-finished:
		case <-time.After(browserHeaderTimeout):
		}
	}()

	select {
	case outcome := <-outcomes:
		return outcome, outcome.err
	case exit := <-finished:
		waited = true

		return browserOutcome{}, fmt.Errorf("lightmap browser exited before result: %w: %s", exit.err, logs.String())
	case <-ctx.Done():
		return browserOutcome{}, ctx.Err()
	}
}

func startBakeServer(ctx context.Context, asset *lightmap.Asset, parameters []byte) (*http.Server, string, <-chan browserOutcome, error) {
	var token [24]byte
	if _, err := rand.Read(token[:]); err != nil {
		return nil, "", nil, err
	}

	prefix := "/" + hex.EncodeToString(token[:]) + "/"
	document := asset.Document()
	resources := map[string][]byte{
		browserShaderName:     []byte(lightmapkernel.Source),
		browserParametersName: parameters,
		"nodes.bin":           document.Nodes,
		"triangles.bin":       document.Triangles,
		"receivers.bin":       document.Receivers,
		"padding.bin":         document.Padding,
		browserScriptName:     browserScript,
	}
	outcomes := make(chan browserOutcome, 1)
	handler := browserHandler(prefix, resources, int(document.Width)*int(document.Height)*4, outcomes)

	listener, err := (&net.ListenConfig{}).Listen(ctx, "tcp", "127.0.0.1:0")
	if err != nil {
		return nil, "", nil, err
	}

	server := &http.Server{
		Handler:           handler,
		ReadHeaderTimeout: browserHeaderTimeout,
		ReadTimeout:       browserTransferTimeout,
		WriteTimeout:      browserTransferTimeout,
		MaxHeaderBytes:    browserHeaderBytes,
	}

	startBrowserServer(server, listener)

	return server, "http://" + listener.Addr().String() + prefix, outcomes, nil
}

func startBrowserServer(server *http.Server, listener net.Listener) {
	go func() { _ = server.Serve(listener) }()
}

func browserHandler(prefix string, resources map[string][]byte, size int, outcomes chan<- browserOutcome) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if !strings.HasPrefix(request.URL.Path, prefix) {
			http.NotFound(writer, request)

			return
		}

		name := strings.TrimPrefix(request.URL.Path, prefix)

		writer.Header().Set("Cache-Control", "no-store")
		writer.Header().Set("X-Content-Type-Options", "nosniff")

		switch {
		case request.Method == http.MethodGet && name == "":
			writer.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = io.WriteString(
				writer,
				"<!doctype html><meta charset=utf-8><title>Karty offline lightmap bake</title><script src=\"bake.js\" defer></script>",
			)
		case request.Method == http.MethodGet && resources[name] != nil:
			writer.Header().Set("Content-Type", "application/octet-stream")

			if name == browserScriptName {
				writer.Header().Set("Content-Type", "text/javascript; charset=utf-8")
			}

			_, _ = writer.Write(resources[name])
		case request.Method == http.MethodPost && (name == "result" || name == browserErrorRoute):
			maximum := size
			if name == browserErrorRoute {
				maximum = 8192
			}

			data, err := io.ReadAll(http.MaxBytesReader(writer, request.Body, int64(maximum)))
			if err != nil {
				http.Error(writer, "invalid result size", http.StatusBadRequest)

				return
			}

			outcome := browserOutcome{pixels: data, adapter: request.Header.Get("X-Karty-Adapter")}
			if name == browserErrorRoute {
				outcome.err = fmt.Errorf("%w: %s", errBrowserExecution, data)
			} else if len(data) != size || outcome.adapter == "" || len(outcome.adapter) > 4096 {
				http.Error(writer, "invalid result", http.StatusBadRequest)

				return
			}

			select {
			case outcomes <- outcome:
			default:
			}

			writer.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(writer, request)
		}
	})
}

func findBrowser(explicit string) (string, error) {
	if explicit != "" {
		return exec.LookPath(explicit)
	}

	for _, name := range []string{"chromium", "chromium-browser", "google-chrome", "chrome-headless-shell"} {
		if path, err := exec.LookPath(name); err == nil {
			return path, nil
		}
	}

	return "", errBrowserMissing
}

func startBrowser(ctx context.Context, executable, profile, url string) (*exec.Cmd, *browserLog, error) {
	//nolint:gosec // Explicit installed executable, argument vector without a shell, random owned profile and loopback URL.
	command := exec.CommandContext(ctx, executable, browserArguments(profile, url)...)
	command.Env = browserEnvironment()
	logs := &browserLog{}
	command.Stdout = logs

	command.Stderr = logs
	if err := command.Start(); err != nil {
		return nil, nil, fmt.Errorf("start lightmap browser: %w", err)
	}

	return command, logs, nil
}

func browserArguments(profile, url string) []string {
	return []string{
		"--no-sandbox",
		"--headless",
		"--user-data-dir=" + profile,
		"--use-gl=angle",
		"--use-angle=gl-egl",
		"--enable-gpu",
		"--ignore-gpu-blocklist",
		"--disable-software-rasterizer",
		"--enable-unsafe-webgpu",
		"--use-webgpu-adapter=opengles",
		"--enable-features=WebGPUService",
		"--force-webgpu-compat",
		url,
	}
}

func browserEnvironment() []string {
	result := make([]string, 0, len(os.Environ())+2)
	for _, value := range os.Environ() {
		if !strings.HasPrefix(value, "EGL_PLATFORM=") && !strings.HasPrefix(value, "LIBGL_ALWAYS_SOFTWARE=") {
			result = append(result, value)
		}
	}

	return append(result, "EGL_PLATFORM=surfaceless", "LIBGL_ALWAYS_SOFTWARE=0")
}

// MarshalReport serializes provenance for a separately encoded atlas.
func MarshalReport(result Result, point lightmap.Point, asset *lightmap.Asset) ([]byte, error) {
	return json.MarshalIndent(struct {
		Version              int            `json:"version"`
		ModelSHA256          string         `json:"modelSha256"`
		Scale                float32        `json:"scale"`
		Point                lightmap.Point `json:"point"`
		Adapter              string         `json:"adapter"`
		DurationMilliseconds float64        `json:"durationMilliseconds"`
	}{1, fmt.Sprintf("%x", asset.ModelDigest()), result.Scale, point, result.Adapter, float64(result.Duration) / float64(time.Millisecond)}, "", "  ")
}
