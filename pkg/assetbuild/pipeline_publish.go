//go:build !js

package assetbuild

import (
	"fmt"
	"path"
)

func validateConfiguredPaths(project, output, buildState string, config Config) error {
	if err := validateSourceRoots(project, output, buildState, config.Input); err != nil {
		return err
	}

	for _, pipeline := range config.Pipelines {
		if err := validateSourceRoots(project, output, buildState, pipeline.Input); err != nil {
			return err
		}

		if pipeline.Directory != "" {
			if _, err := confined(output, pipeline.Directory); err != nil {
				return err
			}
		}
	}

	return nil
}

func (run *runner) collectConfiguredPlan(plan *Plan) (Bundle, error) {
	result := make(Bundle)

	for _, pipeline := range plan.pipelines {
		files, err := run.collect(pipeline.published)
		if err != nil {
			return nil, err
		}
		// Validate original paths before prefixing: Join must not normalize away
		// an invalid '..' or absolute name from a producer's bundle.
		if err := validateOutputRoster(files); err != nil {
			return nil, err
		}

		for name, data := range files {
			if pipeline.directory != "" {
				name = path.Join(pipeline.directory, name)
			}

			if _, exists := result[name]; exists {
				return nil, fmt.Errorf("duplicate output owner %q: %w", name, errPlan)
			}

			result[name] = data
		}
	}

	return result, nil
}
