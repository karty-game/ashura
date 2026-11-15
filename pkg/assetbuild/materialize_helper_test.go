//go:build !js

package assetbuild

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

var (
	errMaterializeFixture        = errors.New("invalid Materialize fixture invocation")
	errMaterializeFixtureFailure = errors.New("fixture requested failure")
)

func materializeHelper() int {
	if err := materializeHelperRun(); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)

		return 1
	}

	return 0
}

func materializeHelperRun() error {
	//nolint:gosec // This subprocess only runs when explicitly enabled by its parent test.
	counter, err := os.OpenFile(os.Getenv("KARTY_MATERIALIZE_COUNT"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}

	_, writeErr := counter.WriteString("call\n")

	closeErr := counter.Close()
	if writeErr != nil || closeErr != nil {
		return errors.Join(writeErr, closeErr)
	}

	mode := os.Getenv("KARTY_MATERIALIZE_MODE")
	if mode == "timeout" {
		time.Sleep(time.Minute)
	}

	if mode == "fail" {
		return errMaterializeFixtureFailure
	}

	if len(os.Args) < 2 {
		return errMaterializeFixture
	}

	flags := flag.NewFlagSet("fixture Materialize", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	output := flags.String("output", "", "")
	maps := flags.String("only", "", "")
	normalFormat := flags.String("normal-format", "", "")
	format := flags.String("format", "", "")
	preset := flags.String("preset", "", "")
	verbose := flags.Bool("verbose", false, "")
	_ = flags.Bool("seamless", false, "")
	_ = flags.Bool("no-seamless", false, "")
	_ = flags.Float64("normal-strength", 0, "")
	_ = flags.Float64("height-contrast", 0, "")

	_ = flags.Float64("ao-depth-scale", 0, "")
	if err := flags.Parse(os.Args[2:]); err != nil {
		return err
	}

	if *output == "" || *normalFormat != "opengl" || *format != "png" || *preset != "brick" || !*verbose || flags.NArg() != 0 {
		return errMaterializeFixture
	}

	//nolint:gosec // The parent passes only its temporary, staged input image.
	encoded, err := os.ReadFile(os.Args[1])
	if err != nil {
		return err
	}

	input, err := png.Decode(bytes.NewReader(encoded))
	if err != nil {
		return err
	}

	if err := materializeHelperMaps(*output, *maps, mode, input.Bounds()); err != nil {
		return err
	}

	switch mode {
	case "software":
		_, _ = fmt.Fprintln(os.Stdout, "GPU: llvmpipe fixture (Cpu, backend Vulkan)")
	case "unknown":
		_, _ = fmt.Fprintln(os.Stdout, "GPU: unknown device")
	default:
		_, _ = fmt.Fprintln(os.Stdout, "GPU: fixture discrete adapter (DiscreteGpu, backend Vulkan)")
	}

	return nil
}

func materializeHelperMaps(output, maps, mode string, bounds image.Rectangle) error {
	if err := os.MkdirAll(output, 0o700); err != nil {
		return err
	}

	for role := range strings.SplitSeq(maps, ",") {
		if mode == "missing" && role == UsageAO {
			continue
		}

		pixel := color.NRGBA{R: 128, G: 128, B: 255, A: 255}
		if role == UsageAO {
			pixel = color.NRGBA{R: 170, G: 170, B: 170, A: 255}
		}

		width := bounds.Dx()
		if mode == "dimensions" {
			width++
		}

		value := image.NewNRGBA(image.Rect(0, 0, width, bounds.Dy()))
		for row := range bounds.Dy() {
			for column := range width {
				value.SetNRGBA(column, row, pixel)
			}
		}

		var encoded bytes.Buffer
		if err := png.Encode(&encoded, value); err != nil {
			return err
		}

		if err := os.WriteFile(filepath.Join(output, "input_"+role+".png"), encoded.Bytes(), 0o600); err != nil {
			return err
		}
	}

	return nil
}
