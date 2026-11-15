//nolint:mnd // Nine cells, center alignment and the image-mask record mode are explicit geometry constants.
package ui

func (v *View) paintPanelBackground(node *Node) {
	background := node.panelBackground
	if background.Image == nil || node.rect.W <= 0 || node.rect.H <= 0 {
		return
	}

	tint := background.Tint
	if tint == (Color{}) {
		tint = Color{1, 1, 1, 1}
	}

	style := v.part(node, PanelBackgroundPart, PartStyle{Fill: tint, Radius: node.style.Radius})
	if background.Slice == nil {
		destination, source := fitPanelImage(node.rect, background.Image, background.Fit)
		v.panelImagePiece(node, destination, source, style)

		return
	}

	v.paintNineSlice(node, background.Image, *background.Slice, style)
}

func fitPanelImage(bounds Rect, image *ImageAsset, fit ImageFit) (Rect, Rect) {
	width, height := float32(image.width), float32(image.height)

	sourceUV := image.uv
	if fit == StretchImage {
		return bounds, sourceUV
	}

	if fit == ContainImage {
		scale := min(bounds.W/width, bounds.H/height)
		width, height = width*scale, height*scale

		return Rect{X: bounds.X + (bounds.W-width)/2, Y: bounds.Y + (bounds.H-height)/2, W: width, H: height}, sourceUV
	}

	scale := max(bounds.W/width, bounds.H/height)
	visibleWidth, visibleHeight := bounds.W/scale/width, bounds.H/scale/height
	sourceUV.X += sourceUV.W * (1 - visibleWidth) / 2
	sourceUV.Y += sourceUV.H * (1 - visibleHeight) / 2
	sourceUV.W *= visibleWidth
	sourceUV.H *= visibleHeight

	return bounds, sourceUV
}

func (v *View) paintNineSlice(node *Node, image *ImageAsset, slice NineSlice, style PartStyle) {
	scale := slice.Scale
	if scale == 0 {
		scale = 1
	}

	left, right := fitPanelBorders(slice.Left*scale, slice.Right*scale, node.rect.W)
	top, bottom := fitPanelBorders(slice.Top*scale, slice.Bottom*scale, node.rect.H)
	positionsX := [4]float32{node.rect.X, node.rect.X + left, node.rect.X + node.rect.W - right, node.rect.X + node.rect.W}
	positionsY := [4]float32{node.rect.Y, node.rect.Y + top, node.rect.Y + node.rect.H - bottom, node.rect.Y + node.rect.H}
	positionsX[2], positionsY[2] = max(positionsX[1], positionsX[2]), max(positionsY[1], positionsY[2])
	sourceX := [4]float32{0, slice.Left, float32(image.width) - slice.Right, float32(image.width)}
	sourceY := [4]float32{0, slice.Top, float32(image.height) - slice.Bottom, float32(image.height)}

	for row := range 3 {
		for column := range 3 {
			if row == 1 && column == 1 && slice.BorderOnly {
				continue
			}

			bounds := Rect{
				X: positionsX[column],
				Y: positionsY[row],
				W: positionsX[column+1] - positionsX[column],
				H: positionsY[row+1] - positionsY[row],
			}

			source := Rect{X: sourceX[column], Y: sourceY[row], W: sourceX[column+1] - sourceX[column], H: sourceY[row+1] - sourceY[row]}
			if bounds.W <= 0 || bounds.H <= 0 || source.W <= 0 || source.H <= 0 {
				continue
			}

			sourceUV := Rect{
				X: image.uv.X + source.X/float32(image.width)*image.uv.W,
				Y: image.uv.Y + source.Y/float32(image.height)*image.uv.H,
				W: source.W / float32(image.width) * image.uv.W,
				H: source.H / float32(image.height) * image.uv.H,
			}
			v.panelImagePiece(node, bounds, sourceUV, style)
		}
	}
}

func fitPanelBorders(first, last, available float32) (float32, float32) {
	if total := first + last; total > available {
		scale := available / total

		first *= scale

		return first, max(0, available-first)
	}

	return first, last
}

func (v *View) panelImagePiece(node *Node, bounds, uv Rect, style PartStyle) {
	node.paint = append(node.paint, paintRecord{
		rect: bounds, clip: node.clip.Intersect(node.rect), color: style.Fill, uv: uv,
		border: Color(node.rect.Array()), radius: style.Radius, mode: 7, texture: node.panelBackground.Image.page,
	})
}
