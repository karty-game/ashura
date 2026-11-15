//go:build !js

package impostorbake

const mipScale = 2

func atlasLevels(options Options) ([][]byte, [][]byte) {
	colorLevels, normalLevels := make([][]byte, options.MipLevels), make([][]byte, options.MipLevels)

	cell := options.Resolution + paddingSides*options.Padding
	for level := range options.MipLevels {
		width, height := (cell*options.columns())>>level, (cell*options.rows())>>level
		colorLevels[level], normalLevels[level] = make([]byte, width*height*channels), make([]byte, width*height*channels)
	}

	return colorLevels, normalLevels
}

func packView(colorLevels, normalLevels [][]byte, colorPixels, normalPixels []byte, azimuth, elevation int, options Options) {
	resolution, padding := options.Resolution, options.Padding
	for level := range options.MipLevels {
		dilate(colorPixels, normalPixels, resolution)
		cell := resolution + padding*paddingSides

		width := len(colorLevels[level]) / (cell * options.rows() * channels)
		for row := range cell {
			for column := range cell {
				source := (min(resolution-1, max(0, row-padding))*resolution + min(resolution-1, max(0, column-padding))) * channels
				target := ((elevation*cell+row)*width + azimuth*cell + column) * channels
				copy(colorLevels[level][target:target+channels], colorPixels[source:source+channels])
				copy(normalLevels[level][target:target+channels], normalPixels[source:source+channels])
			}
		}

		if level+1 < options.MipLevels {
			colorPixels, normalPixels = reduce(colorPixels, normalPixels, resolution)
			resolution, padding = resolution/mipScale, padding/mipScale
		}
	}
}

func reduce(colorPixels, normalPixels []byte, size int) ([]byte, []byte) {
	resultSize := size / mipScale

	colors, normals := make([]byte, resultSize*resultSize*channels), make([]byte, resultSize*resultSize*channels)
	for row := range resultSize {
		for column := range resultSize {
			var (
				colorSum, normalSum vector
				coverage, depth     float64
			)

			for deltaRow := range 2 {
				for deltaColumn := range 2 {
					source := ((row*2+deltaRow)*size + column*2 + deltaColumn) * channels
					weight := float64(colorPixels[source+3]) / byteMaximum

					coverage += weight
					for channel := range 3 {
						colorSum[channel] += toLinear(float64(colorPixels[source+channel])/byteMaximum) * weight
						normalSum[channel] += (float64(normalPixels[source+channel])/byteMaximum*2 - 1) * weight
					}

					depth += float64(normalPixels[source+3]) / byteMaximum * weight
				}
			}

			if coverage <= 0 {
				continue
			}

			target := (row*resultSize + column) * channels
			normalSum = unit(normalSum)

			for channel := range 3 {
				colors[target+channel] = quantize(toSRGB(colorSum[channel] / coverage))
				normals[target+channel] = quantize(normalSum[channel]*.5 + .5)
			}

			colors[target+3] = quantize(coverage / 4)
			normals[target+3] = quantize(depth / coverage)
		}
	}

	return colors, normals
}

// Dilate only shading/depth channels. Coverage stays unchanged so transparent
// texels never become geometry and silhouette filtering cannot introduce black.
func dilate(colors, normals []byte, size int) {
	visited := make([]bool, size*size)
	queue := make([]int, 0, size*size)

	for index := range visited {
		if colors[index*channels+3] != 0 {
			visited[index] = true
			queue = append(queue, index)
		}
	}

	for cursor := 0; cursor < len(queue); cursor++ {
		source := queue[cursor]

		row, column := source/size, source%size
		for _, delta := range [][2]int{{-1, 0}, {1, 0}, {0, -1}, {0, 1}} {
			targetRow, targetColumn := row+delta[0], column+delta[1]
			if targetRow < 0 || targetColumn < 0 || targetRow >= size || targetColumn >= size {
				continue
			}

			target := targetRow*size + targetColumn
			if visited[target] {
				continue
			}

			visited[target] = true
			queue = append(queue, target)
			copy(colors[target*channels:target*channels+3], colors[source*channels:source*channels+3])
			copy(normals[target*channels:target*channels+channels], normals[source*channels:source*channels+channels])
		}
	}
}
