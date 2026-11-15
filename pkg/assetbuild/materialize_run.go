//go:build !js

package assetbuild

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/png"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	materializeLogLimit  = 64 << 10
	materializeWaitDelay = 2 * time.Second
)

func runMaterialize(ctx *Context, resource Resource, options MaterializeOptions) (Result, error) {
	set, err := imageInput(ctx, resource.Ref)
	if err != nil {
		return nil, err
	}

	if len(set.Images) != 1 || len(set.Regions) != 0 || set.Images[0].Atlas || set.Images[0].Usage != UsageAlbedo {
		return nil, fmt.Errorf("Materialize requires one standalone albedo image: %w", errMaterialize)
	}

	executable, err := materializeExecutable(ctx, options)
	if err != nil {
		return nil, err
	}

	stage, err := os.MkdirTemp("", "karty-materialize-")
	if err != nil {
		return nil, fmt.Errorf("stage Materialize: %w", err)
	}
	defer func() { _ = os.RemoveAll(stage) }()

	value := set.Images[0]
	if err := materializeInput(stage, value); err != nil {
		return nil, err
	}

	if err := executeMaterialize(ctx, executable, stage, options); err != nil {
		return nil, err
	}

	result := make(Result, len(options.Maps))
	for _, role := range options.Maps {
		generated, readErr := readMaterializeMap(stage, resource.ID, role, value)
		if readErr != nil {
			return nil, readErr
		}

		data, encodeErr := EncodeImageSet(ImageSet{Images: []Image{generated}})
		if encodeErr != nil {
			return nil, encodeErr
		}

		result[role] = data
	}

	return result, nil
}

func materializeInput(stage string, value Image) error {
	base := &image.NRGBA{Pix: value.Levels[0], Stride: value.Width * imageChannels, Rect: image.Rect(0, 0, value.Width, value.Height)}

	var encoded bytes.Buffer
	if err := png.Encode(&encoded, base); err != nil {
		return fmt.Errorf("encode Materialize input: %w", err)
	}

	if err := os.WriteFile(filepath.Join(stage, "input.png"), encoded.Bytes(), 0o600); err != nil {
		return fmt.Errorf("write Materialize input: %w", err)
	}

	return nil
}

func readMaterializeMap(stage, identity, role string, source Image) (Image, error) {
	data, err := readProjectFile(stage, "output/input_"+role+".png")
	if err != nil {
		return Image{}, fmt.Errorf("Materialize %s output: %w", role, err)
	}

	stem := materializeMapIdentity(identity, role)

	generated, err := DecodeImage(data, AssetName(stem), stem+".png", role)
	if err != nil {
		return Image{}, fmt.Errorf("Materialize %s pixels: %w", role, err)
	}

	if generated.Width != source.Width || generated.Height != source.Height {
		return Image{}, fmt.Errorf("Materialize %s dimensions must match input: %w", role, errMaterialize)
	}

	return generated, nil
}

func executeMaterialize(ctx *Context, executable, stage string, options MaterializeOptions) error {
	toolContext, cancel := context.WithTimeout(ctx.Context, options.Timeout)
	defer cancel()

	//nolint:gosec // The tracked, verified executable uses fixed argv and never passes through a shell.
	command := exec.CommandContext(toolContext, executable, materializeArguments(stage, options)...)
	command.Dir = stage
	command.Env = materializeEnvironment(ctx, options.Environment)
	command.WaitDelay = materializeWaitDelay
	output := &materializeLog{}

	command.Stdout, command.Stderr = output, output
	if err := command.Run(); err != nil {
		if toolContext.Err() != nil {
			return fmt.Errorf("Materialize stopped: %w", toolContext.Err())
		}

		return fmt.Errorf("Materialize failed (%s): %w", output.String(), err)
	}

	adapter, err := materializeAdapter(output.String(), options.AllowSoftware)
	if err != nil {
		return err
	}

	if ctx.runner.log != nil {
		_, _ = fmt.Fprintf(ctx.runner.log, "Materialize GPU: %s\n", adapter)
	}

	return nil
}

func materializeArguments(stage string, options MaterializeOptions) []string {
	arguments := []string{
		"input.png", "--output", filepath.Join(stage, "output"), "--only", strings.Join(options.Maps, ","),
		"--normal-format", "opengl", "--format", "png", "--preset", options.Preset, "--verbose",
	}
	if options.Seamless {
		arguments = append(arguments, "--seamless")
	} else {
		arguments = append(arguments, "--no-seamless")
	}

	for _, override := range []struct {
		name  string
		value *float64
	}{
		{"--normal-strength", options.NormalStrength}, {"--height-contrast", options.HeightContrast}, {"--ao-depth-scale", options.AODepthScale},
	} {
		if override.value != nil {
			arguments = append(arguments, override.name, strconv.FormatFloat(*override.value, 'g', -1, 32))
		}
	}

	return arguments
}

func materializeEnvironment(ctx *Context, overrides map[string]string) []string {
	environment := make(map[string]string)

	for name := range strings.FieldsSeq(
		"PATH HOME USERPROFILE SYSTEMROOT WINDIR TEMP TMP XDG_RUNTIME_DIR XDG_CACHE_HOME XDG_CONFIG_HOME XDG_DATA_HOME " +
			"DISPLAY WAYLAND_DISPLAY EGL_PLATFORM LIBGL_ALWAYS_SOFTWARE MESA_LOADER_DRIVER_OVERRIDE MESA_VK_DEVICE_SELECT DRI_PRIME " +
			"MATERIALIZE_GPU_BACKEND MATERIALIZE_LOG WGPU_BACKEND WGPU_ADAPTER_NAME WGPU_POWER_PREF WGPU_DX12_COMPILER " +
			"WGPU_GLES_MINOR_VERSION VK_ICD_FILENAMES VK_DRIVER_FILES VK_ADD_DRIVER_FILES VK_INSTANCE_LAYERS VK_LAYER_PATH " +
			"VK_LOADER_DRIVERS_SELECT VK_LOADER_DRIVERS_DISABLE LD_LIBRARY_PATH LD_PRELOAD DYLD_LIBRARY_PATH DYLD_FALLBACK_LIBRARY_PATH",
	) {
		if _, exists := overrides[name]; !exists {
			if value := ctx.Getenv(name); value != "" {
				environment[name] = value
			}
		}
	}

	maps.Copy(environment, overrides)

	keys := make([]string, 0, len(environment))
	for name := range environment {
		keys = append(keys, name)
	}

	slices.Sort(keys)

	result := make([]string, 0, len(keys))
	for _, name := range keys {
		result = append(result, name+"="+environment[name])
	}

	return result
}

func materializeAdapter(output string, allowSoftware bool) (string, error) {
	for line := range strings.SplitSeq(output, "\n") {
		if !strings.HasPrefix(line, "GPU: ") {
			continue
		}

		adapter := strings.TrimSpace(strings.TrimPrefix(line, "GPU: "))

		lower := strings.ToLower(adapter)
		for _, marker := range []string{"llvmpipe", "lavapipe", "swiftshader", "softpipe", "software", "(cpu,"} {
			if strings.Contains(lower, marker) {
				if allowSoftware {
					return adapter, nil
				}

				return "", fmt.Errorf("Materialize selected software adapter %q: %w", adapter, errMaterialize)
			}
		}

		if strings.Contains(adapter, "(IntegratedGpu,") || strings.Contains(adapter, "(DiscreteGpu,") ||
			strings.Contains(adapter, "(VirtualGpu,") {
			return adapter, nil
		}

		return "", fmt.Errorf("Materialize adapter identity %q is not a recognized GPU: %w", adapter, errMaterialize)
	}

	return "", fmt.Errorf("Materialize verbose output omitted GPU identity: %w", errMaterialize)
}

type materializeLog struct {
	mu   sync.Mutex
	data []byte
}

func (output *materializeLog) Write(data []byte) (int, error) {
	output.mu.Lock()
	defer output.mu.Unlock()

	remaining := materializeLogLimit - len(output.data)
	output.data = append(output.data, data[:min(len(data), remaining)]...)

	return len(data), nil
}

func (output *materializeLog) String() string {
	output.mu.Lock()
	defer output.mu.Unlock()

	return string(output.data)
}
