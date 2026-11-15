//go:build !js

package lightmapbuild

import (
	"context"
	"crypto/rand"
	_ "embed"
	"encoding/hex"
	"net"
	"net/http"
	"os"
	"time"

	"github.com/karty-game/karty-webgpu/internal/lightmapkernel"
	"github.com/karty-game/karty-webgpu/pkg/lightmap"
	"github.com/karty-game/karty-webgpu/pkg/texture"
)

//go:embed rnm_browser.js
var rnmBrowserScript []byte

// BakeRNMBrowser executes the same module, packed parameters and row schedule
// as RNMBaker, then reads back all three completed maps. Geometry is immutable;
// no browser-side preparation, CPU visibility fallback or partial result occurs.
func BakeRNMBrowser(ctx context.Context, asset *lightmap.Asset, scene lightmap.RNMScene, options BrowserOptions) (RNMResult, error) {
	started := time.Now()

	if ctx == nil || asset == nil {
		return RNMResult{}, errBrowserArguments
	}

	if err := ctx.Err(); err != nil {
		return RNMResult{}, err
	}

	parameters, points, scale, normalized, err := lightmapkernel.RNMParameters(asset.Document(), scene)
	if err != nil {
		return RNMResult{}, err
	}

	executable, err := findBrowser(options.Executable)
	if err != nil {
		return RNMResult{}, err
	}

	server, url, outcomes, err := startRNMBakeServer(ctx, asset, parameters[:], points)
	if err != nil {
		return RNMResult{}, err
	}

	defer func() { _ = server.Close() }()

	profile, err := os.MkdirTemp("", "karty-rnm-browser-")
	if err != nil {
		return RNMResult{}, err
	}

	defer func() { _ = os.RemoveAll(profile) }()

	command, logs, err := startBrowser(ctx, executable, profile, url)
	if err != nil {
		return RNMResult{}, err
	}

	outcome, err := awaitBrowser(ctx, command, logs, outcomes)
	if err != nil {
		return RNMResult{}, err
	}

	width, height := asset.Size()
	result := RNMResult{
		Scale:    scale,
		Adapter:  outcome.adapter,
		Duration: time.Since(started),
		Samples:  normalized.Samples,
		Bounces:  normalized.Bounces,
	}

	if len(asset.Document().ReceiverGIIndex) > 0 {
		result.Encoding = texture.RNMRGBE
	}

	const pixelBytes = 4

	size := int(width) * int(height) * pixelBytes
	for i := range result.Textures {
		result.Textures[i] = texture.Data{
			Width:      int(width),
			Height:     int(height),
			ColorSpace: texture.Linear,
			Levels:     [][]byte{outcome.pixels[i*size : (i+1)*size]},
		}
	}

	return result, nil
}

func startRNMBakeServer(
	ctx context.Context,
	asset *lightmap.Asset,
	parameters, points []byte,
) (*http.Server, string, <-chan browserOutcome, error) {
	var token [24]byte
	if _, err := rand.Read(token[:]); err != nil {
		return nil, "", nil, err
	}

	prefix := "/" + hex.EncodeToString(token[:]) + "/"
	resources := map[string][]byte{
		browserShaderName:     []byte(lightmapkernel.RNMSource),
		browserParametersName: parameters,
		"scene.bin":           asset.Document().GPUData,
		"points.bin":          points,
		"bake.js":             rnmBrowserScript,
	}
	outcomes := make(chan browserOutcome, 1)
	width, height := asset.Size()

	listener, err := (&net.ListenConfig{}).Listen(ctx, "tcp", "127.0.0.1:0")
	if err != nil {
		return nil, "", nil, err
	}

	server := &http.Server{
		Handler:           browserHandler(prefix, resources, int(width)*int(height)*12, outcomes),
		ReadHeaderTimeout: browserHeaderTimeout,
		ReadTimeout:       browserTransferTimeout,
		WriteTimeout:      browserTransferTimeout,
		MaxHeaderBytes:    browserHeaderBytes,
	}
	startBrowserServer(server, listener)

	return server, "http://" + listener.Addr().String() + prefix, outcomes, nil
}
