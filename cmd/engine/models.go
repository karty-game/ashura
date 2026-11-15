//go:build !js

package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/karty-game/karty-webgpu/internal/modelbuild"
)

var errModelArgs = errors.New("usage: engine assets model --input source.glb --output prepared.kmodel")

func buildModel(args []string) error {
	flags := flag.NewFlagSet("assets model", flag.ContinueOnError)
	input := flags.String("input", "", "source static GLB 2.0")

	output := flags.String("output", "", "prepared GPU-ready model output")
	if err := flags.Parse(args); err != nil {
		return err
	}

	if flags.NArg() != 0 || *input == "" || *output == "" {
		return errModelArgs
	}

	started := time.Now()

	result, err := modelbuild.Prepare(*input, *output)
	if err != nil {
		return err
	}

	_, _ = fmt.Fprintf(
		os.Stdout,
		"Prepared model %s -> %s (static-v1, %d primitives, %d textures, vertices=%d bytes, indices=%d bytes, textures=%d bytes, total=%d bytes, %s)\n",
		*input,
		*output,
		result.Parts,
		result.Textures,
		result.VertexBytes,
		result.IndexBytes,
		result.TextureBytes,
		len(result.Data),
		time.Since(started),
	)

	return nil
}
