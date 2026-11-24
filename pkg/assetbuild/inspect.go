//go:build !js

package assetbuild

import (
	"context"
	"errors"
	"fmt"
	"io"
	"maps"
	"path"
	"path/filepath"
	"slices"
	"strings"
)

// Inspect runs the recipe's planning stages and writes a source and image
// catalog preview to options.Log. It does not run actions or write project
// assets or build state. Configure a Log writer to receive the preview.
func Inspect(ctx context.Context, config Config, options Options) (err error) {
	if err := ctx.Err(); err != nil {
		return err
	}

	run, err := newRunner(config, options)
	if err != nil {
		return err
	}

	plan, err := run.makePlan(config, options, true)
	if plan != nil {
		defer func() { err = errors.Join(err, closePlanCleanups(plan)) }()
	}

	if err != nil {
		return err
	}

	if err := ctx.Err(); err != nil {
		return err
	}

	return writeInspection(options.Log, plan)
}

type inspectedImage struct {
	source           imageSource
	atlas            string
	reference        string
	publishedTexture string
}

type atlasInspection struct {
	name      string
	selected  int
	unmatched []string
}

type pipelineInspection struct {
	name    string
	globs   map[string][]string
	images  []*inspectedImage
	byRef   map[Ref][]*inspectedImage
	skipped map[string]bool
	atlases []atlasInspection
	loader  string
}

func newPipelineInspection(name string) *pipelineInspection {
	return &pipelineInspection{
		name: name, globs: make(map[string][]string),
		byRef: make(map[Ref][]*inspectedImage), skipped: make(map[string]bool),
	}
}

func (plan *Plan) inspectImage(ref Ref, source imageSource) {
	if info := plan.inspection; info != nil {
		image := &inspectedImage{source: source}
		info.images = append(info.images, image)
		info.byRef[ref] = []*inspectedImage{image}
	}
}

func (plan *Plan) carryInspectedImages(from, to Ref) {
	if info := plan.inspection; info != nil {
		info.byRef[to] = info.byRef[from]
	}
}

func (plan *Plan) inspectAtlas(options AtlasOptions, selected []Resource) {
	info := plan.inspection
	if info == nil {
		return
	}

	atlas := atlasInspection{name: options.Name, selected: len(selected)}
	for _, pattern := range options.Include {
		if !slices.ContainsFunc(selected, func(resource Resource) bool {
			matches, _ := MatchAsset(pattern, resource.ID) // Already validated by Atlas.

			return matches
		}) {
			atlas.unmatched = append(atlas.unmatched, pattern)
		}
	}

	info.atlases = append(info.atlases, atlas)
}

func (plan *Plan) inspectAtlasImages(to Ref, inputs []Ref, name string) {
	if info := plan.inspection; info != nil {
		for _, from := range inputs {
			for _, image := range info.byRef[from] {
				image.atlas = name
				info.byRef[to] = append(info.byRef[to], image)
			}
		}
	}
}

// DescribeImageFields reports the catalog fields for known source images
// consumed by a publisher. loader is an example call returning the catalog.
// This annotation has no effect during a build.
func (plan *Plan) DescribeImageFields(inputs []Ref, loader string) {
	if info := plan.inspection; info != nil {
		info.loader = loader
		for _, ref := range inputs {
			for _, image := range info.byRef[plan.scopedRef(ref)] {
				image.reference = "catalog." + image.source.name
			}
		}
	}
}

func (plan *Plan) inspectTextureAccessors(inputs []Ref) {
	if info := plan.inspection; info != nil {
		for _, ref := range inputs {
			for _, image := range info.byRef[ref] {
				if image.atlas != "" {
					image.reference = "assets." + image.source.name + "Region()"

					continue
				}

				image.reference = "assets." + image.source.name + "()"
				image.publishedTexture = textureOutput(image.source.relative)
			}
		}
	}
}

func writeInspection(out io.Writer, plan *Plan) error {
	if out == nil {
		return nil
	}

	relative, err := filepath.Rel(plan.Project, plan.Output)
	if err != nil {
		return err
	}

	var report strings.Builder

	env := plan.Environment
	_, _ = fmt.Fprintf(&report, "Asset inspection (%s, profile %s, target %s/%s)\n", env.Phase, env.Profile, env.TargetOS, env.TargetArch)

	for _, pipeline := range plan.pipelines {
		writePipelineInspection(&report, pipeline, path.Join(filepath.ToSlash(relative), pipeline.directory))
	}

	_, err = io.WriteString(out, report.String())

	return err
}

func writePipelineInspection(report *strings.Builder, plan *Plan, output string) {
	info := plan.inspection

	name := info.name
	if name == "" {
		name = "default"
	}

	_, _ = fmt.Fprintf(report, "\nPipeline %s → %s\n", name, output)

	writeInspectedSources(report, info)

	for _, atlas := range info.atlases {
		_, _ = fmt.Fprintf(report, "  Atlas %q: %d selected\n", atlas.name, atlas.selected)
		for _, pattern := range atlas.unmatched {
			_, _ = fmt.Fprintf(
				report,
				"  Warning: atlas %q pattern %q matched no images; use extensionless image IDs within the input root\n",
				atlas.name,
				pattern,
			)
		}
	}

	if info.loader != "" {
		_, _ = fmt.Fprintf(report, "  Load catalog with %s\n", info.loader)
	}

	for _, image := range info.images {
		writeInspectedImage(report, image, output)
	}

	discoveries := len(plan.steps) - len(plan.actions)
	if discoveries > 0 {
		_, _ = fmt.Fprintf(report, "  Discovery steps that require a build: %d\n", discoveries)
	}
}

func writeInspectedSources(report *strings.Builder, info *pipelineInspection) {
	shown := make(map[string]bool, len(info.images)+len(info.skipped))
	for _, image := range info.images {
		shown[image.source.path] = true
	}

	for _, file := range slices.Sorted(maps.Keys(info.skipped)) {
		_, _ = fmt.Fprintf(report, "  Skipped by PNG/JPEG importer: %q (unsupported file extension)\n", file)
		shown[file] = true
	}

	for _, pattern := range slices.Sorted(maps.Keys(info.globs)) {
		files := info.globs[pattern]
		if len(files) == 0 {
			_, _ = fmt.Fprintf(report, "  Warning: input pattern %q matched no files\n", pattern)

			continue
		}

		_, _ = fmt.Fprintf(report, "  Input %q matched %d\n", pattern, len(files))
		for _, file := range files {
			if !shown[file] {
				_, _ = fmt.Fprintf(report, "    %q\n", file)
				shown[file] = true
			}
		}
	}
}

func writeInspectedImage(report *strings.Builder, image *inspectedImage, output string) {
	_, _ = fmt.Fprintf(report, "  %q (ID %q, %s)", image.source.path, image.source.identity, image.source.usage)
	if image.atlas != "" {
		_, _ = fmt.Fprintf(report, " → atlas %q", image.atlas)
	}

	if image.publishedTexture != "" {
		_, _ = fmt.Fprintf(report, " → %q", path.Join(output, image.publishedTexture))
	}

	if image.reference != "" {
		_, _ = fmt.Fprintf(report, " → %s", image.reference)
	}

	report.WriteByte('\n')
}
