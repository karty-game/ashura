//go:build !js

package main

import (
	"fmt"
	"strconv"
	"strings"
)

func parseAssetRunArgs(args []string) ([]string, assetOptions, error) {
	options := assetOptions{profile: "dev", target: "js/wasm"}
	remaining := make([]string, 0, len(args))

	for len(args) > 0 {
		argument := args[0]
		args = args[1:]

		switch {
		case argument == "--profile":
			if len(args) == 0 {
				return nil, assetOptions{}, fmt.Errorf("--profile requires a name: %w", errUsage)
			}

			options.profile, args = args[0], args[1:]
			if options.profile == "" || strings.HasPrefix(options.profile, "-") {
				return nil, assetOptions{}, fmt.Errorf("--profile requires a name: %w", errUsage)
			}
		case strings.HasPrefix(argument, "--profile="):
			options.profile = strings.TrimPrefix(argument, "--profile=")
			if options.profile == "" {
				return nil, assetOptions{}, fmt.Errorf("--profile requires a name: %w", errUsage)
			}
		case argument == "--no-cache":
			options.noCache = true
		case strings.HasPrefix(argument, "--no-cache="):
			value, err := strconv.ParseBool(strings.TrimPrefix(argument, "--no-cache="))
			if err != nil {
				return nil, assetOptions{}, fmt.Errorf("--no-cache: %w", err)
			}

			options.noCache = value
		default:
			remaining = append(remaining, argument)
		}
	}

	return remaining, options, nil
}
