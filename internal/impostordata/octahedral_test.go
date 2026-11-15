package impostordata

import (
	"encoding/binary"
	"math"
	"testing"
)

func octahedralDocument() Document {
	document := fixtureDocument()
	document.Mode, document.HalfExtents = ModeOctahedral, [3]float32{1, 1, 1}
	document.Azimuths, document.Elevations, document.Columns = 4, 4, 4
	document.AtlasWidth, document.AtlasHeight = 16, 16
	document.ElevationMin, document.ElevationMax = 0, 0
	view := document.Views[0]

	document.Views = make([]View, 16)
	for index := range document.Views {
		document.Views[index] = view
	}

	return document
}

func TestOctahedralV4EnvelopeAndPreparedUniform(t *testing.T) {
	t.Parallel()

	data, err := Encode(octahedralDocument())
	if err != nil {
		t.Fatal(err)
	}

	document, err := Decode(data)
	if err != nil || document.Mode != ModeOctahedral || len(document.Views) != 16 {
		t.Fatalf("octahedral envelope mismatch: %v", err)
	}

	if len(document.Uniform) != 3168 || binary.LittleEndian.Uint32(document.Uniform[76:]) != math.Float32bits(4) {
		t.Fatal("offline descriptor does not match the shader layout")
	}

	for _, obsolete := range []struct {
		magic   string
		version uint32
	}{{"KIMPOST1", 1}, {"KIMPOST2", 2}, {"KIMPOST3", 3}} {
		copy(data[:8], obsolete.magic)
		binary.LittleEndian.PutUint32(data[8:], obsolete.version)

		if _, err := Decode(data); err == nil {
			t.Fatal("admitted obsolete container requiring runtime descriptor conversion", obsolete.version)
		}
	}
}

func TestOctahedralRejectsInvalidGridsAndRemovedModes(t *testing.T) {
	t.Parallel()

	for _, name := range []string{"axial", "planes", "surface", "unknown", "grid", "rectangular", "columns", "views", "elevation", "bounds", "plane", "basis"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			document := octahedralDocument()

			switch name {
			case "axial":
				document.Mode = 1
			case "planes":
				document.Mode = 2
			case "surface":
				document.Mode = 3
			case "unknown":
				document.Mode = ModeLayered + 1
			case "grid":
				document.Azimuths = 9
			case "rectangular":
				document.Elevations = 5
			case "columns":
				document.Columns = 2
			case "views":
				document.Views = document.Views[:15]
			case "elevation":
				document.ElevationMax = 1
			case "bounds":
				document.HalfExtents[0] = 0
			case "plane":
				document.Views[0].PlaneDepth = .1
			case "basis":
				document.Views[0].Up = document.Views[0].Right
			}

			if _, err := Encode(document); err == nil {
				t.Fatalf("accepted malformed octahedral contract: %s", name)
			}
		})
	}
}
