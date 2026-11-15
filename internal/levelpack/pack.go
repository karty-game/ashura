//go:build !js

package levelpack

import (
	"cmp"
	"context"
	"image"
	"slices"
)

const maximumAtlases = 64

type atlas struct {
	charts []chart
	free   []image.Rectangle
	// Reuse the previous buffers after each insertion. Free rectangles form an
	// antichain: only newly split rectangles can introduce containment.
	scratch []image.Rectangle
	split   []int
	size    int
}

// Pack keeps every chunk and scale/encoding family independent. It copies
// integer texel rectangles and changes UV1 only; UV0 and geometry stay exact.
func Pack(ctx context.Context, inputs []Input, maximum int) ([]Output, error) {
	if ctx == nil || len(inputs) == 0 || len(inputs) > 64 || maximum < 16 || maximum > 2048 || maximum&(maximum-1) != 0 {
		return nil, ErrInput
	}

	components, err := charts(inputs)
	if err != nil {
		return nil, err
	}

	slices.SortStableFunc(components, func(first, second chart) int {
		return cmp.Or(cmp.Compare(second.bounds.Dy(), first.bounds.Dy()), cmp.Compare(second.bounds.Dx(), first.bounds.Dx()))
	})

	atlases, err := packCharts(ctx, inputs, components, maximum)
	if err != nil {
		return nil, err
	}

	output := make([]Output, 0, len(atlases))
	for _, value := range atlases {
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		value.shrink()

		packed, err := assemble(inputs, value)
		if err != nil {
			return nil, err
		}

		output = append(output, packed)
	}

	return output, nil
}

func packCharts(ctx context.Context, inputs []Input, components []chart, maximum int) ([]atlas, error) {
	var atlases []atlas

	for _, component := range components {
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		if component.bounds.Dx() > maximum || component.bounds.Dy() > maximum {
			return nil, ErrInput
		}

		placed := false

		for index := range atlases {
			family := inputs[atlases[index].charts[0].input]

			source := inputs[component.input]
			if family.Scale == source.Scale && family.Encoding == source.Encoding && family.Representation == source.Representation &&
				atlases[index].insert(component) {
				placed = true

				break
			}
		}

		if !placed {
			value := atlas{size: maximum, free: []image.Rectangle{image.Rect(0, 0, maximum, maximum)}}
			if !value.insert(component) {
				return nil, ErrInput
			}

			atlases = append(atlases, value)
			if len(atlases) > maximumAtlases {
				return nil, ErrInput
			}
		}
	}

	return atlases, nil
}

func (value *atlas) insert(component chart) bool {
	best, score := -1, int(^uint(0)>>1)

	width, height := component.bounds.Dx(), component.bounds.Dy()
	for index, free := range value.free {
		if width <= free.Dx() && height <= free.Dy() {
			candidate := min(free.Dx()-width, free.Dy()-height)
			if candidate < score {
				best, score = index, candidate
			}
		}
	}

	if best < 0 {
		return false
	}

	component.position = value.free[best].Min
	used := image.Rectangle{Min: component.position, Max: component.position.Add(image.Pt(width, height))}

	next := value.scratch[:0]
	split := value.split[:0]

	for _, free := range value.free {
		if !free.Overlaps(used) {
			next = append(next, free)

			continue
		}

		for _, rectangle := range []image.Rectangle{
			image.Rect(free.Min.X, free.Min.Y, used.Min.X, free.Max.Y),
			image.Rect(used.Max.X, free.Min.Y, free.Max.X, free.Max.Y),
			image.Rect(free.Min.X, free.Min.Y, free.Max.X, used.Min.Y),
			image.Rect(free.Min.X, used.Max.Y, free.Max.X, free.Max.Y),
		} {
			if !rectangle.Empty() && rectangle.In(free) {
				split = append(split, len(next))
				next = append(next, rectangle)
			}
		}
	}

	value.prune(next, split)

	value.charts = append(value.charts, component)

	return true
}

// prune compares only new splits against the established antichain.
func (value *atlas) prune(next []image.Rectangle, split []int) {
	kept := value.free[:0]
	nextSplit := 0

	for index, free := range next {
		contained := false

		isSplit := nextSplit < len(split) && split[nextSplit] == index
		if isSplit {
			nextSplit++

			for other, candidate := range next {
				if other != index && free.In(candidate) && (free != candidate || other < index) {
					contained = true

					break
				}
			}
		} else {
			// Retained rectangles were already mutually non-containing. Compare
			// them only with new splits, preserving the original stable tie rule.
			for _, other := range split {
				candidate := next[other]
				if free.In(candidate) && (free != candidate || other < index) {
					contained = true

					break
				}
			}
		}

		if !contained {
			kept = append(kept, free)
		}
	}

	value.free, value.scratch, value.split = kept, next[:0], split
}

func (value *atlas) shrink() {
	const reductionFactor = 2

	for size := value.size / reductionFactor; size >= 16; size /= reductionFactor {
		candidate := atlas{size: size, free: []image.Rectangle{image.Rect(0, 0, size, size)}}
		fits := true

		for _, component := range value.charts {
			if !candidate.insert(component) {
				fits = false

				break
			}
		}

		if !fits {
			break
		}

		*value = candidate
	}
}
