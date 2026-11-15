//go:build !js

package assetbuild

import (
	"fmt"
	"go/token"
	"io/fs"
	"strings"
)

func validatePluginStages(config Config) error {
	if len(config.Pipelines) != 0 {
		if len(config.Input) != 0 || len(config.Plugins) != 0 || len(config.Import) != 0 || len(config.Process) != 0 ||
			len(config.Export) != 0 {
			return fmt.Errorf("pipelines cannot be combined with shared inputs or plugins: %w", errPlan)
		}

		if err := validatePipelineNames(config.Pipelines); err != nil {
			return err
		}
	}

	if len(config.Plugins) != 0 && (len(config.Import) != 0 || len(config.Process) != 0 || len(config.Export) != 0) {
		return fmt.Errorf("flat Plugins cannot be combined with Import, Process or Export: %w", errPlan)
	}

	return nil
}

func validatePipelineNames(pipelines []Pipeline) error {
	if len(pipelines) > maxPlanSteps {
		return fmt.Errorf("asset pipeline count exceeds graph bound: %w", errPlan)
	}

	names := make(map[string]bool, len(pipelines))
	for _, pipeline := range pipelines {
		if !token.IsIdentifier(pipeline.Name) || names[pipeline.Name] {
			return fmt.Errorf("invalid or duplicate asset pipeline %q: %w", pipeline.Name, errPlan)
		}

		if pipeline.Directory != "" &&
			(pipeline.Directory == "." || !fs.ValidPath(pipeline.Directory) || strings.Contains(pipeline.Directory, `\`)) {
			return fmt.Errorf("invalid asset pipeline directory %q: %w", pipeline.Directory, errPath)
		}

		names[pipeline.Name] = true
	}

	return nil
}

// pipelineDefinition is the admitted recipe form. Historical recipes use an
// empty namespace so normalization preserves their action IDs and cache keys.
type pipelineDefinition struct {
	pipeline  Pipeline
	namespace string
}

func normalizePipelines(config Config) ([]pipelineDefinition, error) {
	if err := validatePluginStages(config); err != nil {
		return nil, err
	}

	if len(config.Pipelines) != 0 {
		definitions := make([]pipelineDefinition, 0, len(config.Pipelines))
		for _, pipeline := range config.Pipelines {
			definitions = append(definitions, pipelineDefinition{pipeline: pipeline, namespace: "pipeline/" + pipeline.Name + "/"})
		}

		return definitions, nil
	}

	pipeline := Pipeline{Input: config.Input, Import: config.Import, Process: config.Process, Output: config.Export}
	if len(config.Plugins) != 0 {
		pipeline.Import = config.Plugins
	}

	return []pipelineDefinition{{pipeline: pipeline}}, nil
}
