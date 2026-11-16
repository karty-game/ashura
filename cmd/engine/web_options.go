//go:build !js

package main

import (
	"fmt"
	"strings"
)

type webOptions struct {
	assets   assetOptions
	compiler string
}

func parseWebOptions(args []string) ([]string, webOptions, error) {
	compiler := automaticWebCompiler
	remaining := make([]string, 0, len(args))

	for index := 0; index < len(args); index++ {
		argument := args[index]
		switch {
		case argument == "--compiler":
			if index+1 >= len(args) {
				return nil, webOptions{}, fmt.Errorf("--compiler requires auto, go or tinygo: %w", errUsage)
			}

			index++
			//nolint:gosec // The explicit index+1 bound above is checked before advancing.
			compiler = args[index]
		case strings.HasPrefix(argument, "--compiler="):
			compiler = strings.TrimPrefix(argument, "--compiler=")
		default:
			remaining = append(remaining, argument)
		}
	}

	if compiler != automaticWebCompiler && compiler != "go" && compiler != tinyGoCompiler {
		return nil, webOptions{}, fmt.Errorf("unsupported browser compiler %q; choose auto, go or tinygo: %w", compiler, errUsage)
	}

	remaining, assets, err := parseAssetRunArgs(remaining)
	if err != nil {
		return nil, webOptions{}, err
	}

	return remaining, webOptions{assets: assets, compiler: compiler}, nil
}
