package impostordata

// LayerLayout describes offline texture regions for three unsigned depth axes.
type LayerLayout struct {
	Width, Height, CellSize, AttributeRows, PairRows uint32
	PairBaseRow, HierarchyBaseRow, NodesPerAxis      uint32
}

const (
	LayeredAxes            = 3
	LayeredColumns         = 8
	MinimumLayerResolution = 32
	MaximumLayerResolution = 256
	MaximumIntervals       = 16
	layerBoundaryCount     = 2
	treeDivisor            = 3
	treeChildren           = 4
	layoutAlignment        = 4
)

// LayeredLayout checks every dimension before deriving numeric texture offsets.
//
//nolint:gosec // Resolution<=256 and dimensions<=4096 bound every derived uint64 value below uint32.
func LayeredLayout(resolution, padding, intervals uint32) (LayerLayout, error) {
	if resolution < MinimumLayerResolution || resolution > MaximumLayerResolution || resolution&(resolution-1) != 0 || padding < 1 ||
		padding > 128 ||
		intervals < 1 ||
		intervals > MaximumIntervals {
		return LayerLayout{}, errFormat
	}

	cell := uint64(resolution) + layerBoundaryCount*uint64(padding)
	width := LayeredColumns * cell
	attributes := (LayeredAxes*layerBoundaryCount*uint64(intervals) + LayeredColumns - 1) / LayeredColumns
	pairs := (LayeredAxes*uint64(intervals) + LayeredColumns - 1) / LayeredColumns
	nodes := (treeChildren*uint64(resolution)*uint64(resolution) - 1) / treeDivisor
	pairBase := attributes * cell
	hierarchyBase := (attributes + pairs) * cell

	height := (hierarchyBase + (LayeredAxes*nodes+width-1)/width + layoutAlignment - 1) / layoutAlignment * layoutAlignment
	if width > maxTextureDimension || height > maxTextureDimension || width*height*8 > MaxBytes {
		return LayerLayout{}, errFormat
	}

	return LayerLayout{
		Width:            uint32(width),
		Height:           uint32(height),
		CellSize:         uint32(cell),
		AttributeRows:    uint32(attributes),
		PairRows:         uint32(pairs),
		PairBaseRow:      uint32(pairBase),
		HierarchyBaseRow: uint32(hierarchyBase),
		NodesPerAxis:     uint32(nodes),
	}, nil
}

// LayeredViews fixes the shared coordinate frames used by offline and GPU paths.
func LayeredViews() []View {
	return []View{
		{Right: [3]float32{0, 0, -1}, Up: [3]float32{0, 1, 0}, Outward: [3]float32{1, 0, 0}},
		{Right: [3]float32{1, 0, 0}, Up: [3]float32{0, 0, -1}, Outward: [3]float32{0, 1, 0}},
		{Right: [3]float32{1, 0, 0}, Up: [3]float32{0, 1, 0}, Outward: [3]float32{0, 0, 1}},
	}
}

func validLayeredGrid(document Document) bool {
	if document.Elevations != LayeredAxes || document.Columns != LayeredColumns || document.MipLevels != 1 || document.ElevationMin != 0 ||
		document.ElevationMax != 0 ||
		len(document.Views) != LayeredAxes {
		return false
	}

	for index, expected := range LayeredViews() {
		if document.Views[index] != expected {
			return false
		}
	}

	layout, err := LayeredLayout(document.Resolution, document.Padding, document.Azimuths)

	return err == nil && document.AtlasWidth == layout.Width && document.AtlasHeight == layout.Height &&
		document.CellSize == layout.CellSize
}
