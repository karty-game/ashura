package impostordata

import (
	"encoding/binary"
	"testing"
)

func layeredDocument() Document {
	document := fixtureDocument()
	document.Mode = ModeLayered
	document.HalfExtents = [3]float32{1, 1, 1}
	document.Azimuths, document.Elevations, document.Columns = 8, 3, 8
	document.Resolution, document.Padding, document.CellSize, document.MipLevels = 32, 4, 40, 1
	layout, _ := LayeredLayout(32, 4, 8)
	document.AtlasWidth, document.AtlasHeight = layout.Width, layout.Height
	document.ElevationMin, document.ElevationMax = 0, 0
	document.Views = LayeredViews()

	return document
}
func TestLayeredLayoutAndV4Admission(t *testing.T) {
	t.Parallel()

	layout, err := LayeredLayout(128, 4, 8)
	if err != nil || layout.Width != 1088 || layout.Height != 1288 || layout.AttributeRows != 6 || layout.PairRows != 3 ||
		layout.PairBaseRow != 816 ||
		layout.HierarchyBaseRow != 1224 ||
		layout.NodesPerAxis != 21845 {
		t.Fatalf("unexpected layout %+v: %v", layout, err)
	}

	document := layeredDocument()

	encoded, err := Encode(document)
	if err != nil {
		t.Fatal(err)
	}

	decoded, err := Decode(encoded)
	if err != nil || decoded.Mode != ModeLayered || len(decoded.Uniform) != 3168 || decoded.Storage != StorageRGBA8 {
		t.Fatalf("layered admission: %v", err)
	}

	copy(encoded[:8], "KIMPOST2")
	binary.LittleEndian.PutUint32(encoded[8:], 2)

	if _, err := Decode(encoded); err == nil {
		t.Fatal("admitted obsolete v2")
	}
}
func TestLayeredRejectsUnboundedLayoutAndIncompatibleMetadata(t *testing.T) {
	t.Parallel()

	for _, values := range [][3]uint32{{31, 4, 8}, {80, 4, 8}, {128, 129, 8}, {128, 4, 17}, {256, 128, 16}, {1 << 31, 4, 8}, {128, 1 << 31, 8}, {128, 4, 1 << 31}} {
		if _, err := LayeredLayout(values[0], values[1], values[2]); err == nil {
			t.Fatalf("accepted invalid layout %v", values)
		}
	}

	for _, name := range []string{"mips", "dimensions", "count", "view", "columns", "grid", "angle"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			document := layeredDocument()

			switch name {
			case "mips":
				document.MipLevels = 2
			case "dimensions":
				document.AtlasHeight++
			case "count":
				document.Azimuths = 17
			case "view":
				document.Views[0].Right[2] = 1
			case "columns":
				document.Columns = 7
			case "grid":
				document.Elevations = 4
			case "angle":
				document.ElevationMax = 1
			}

			if _, err := Encode(document); err == nil {
				t.Fatalf("admitted invalid layered %s", name)
			}
		})
	}
}
