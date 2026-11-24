package lightmapdata_test

import (
	"encoding/binary"
	"math"
	"testing"

	"github.com/karty-game/ashura/internal/lightmapdata"
	"github.com/karty-game/ashura/internal/lightmapfixture"
)

func TestTransportRoundTripPreservesMetadataAndOwnedContiguousPayload(t *testing.T) {
	t.Parallel()

	original := lightmapfixture.TransportDocument()

	encoded, err := lightmapdata.Encode(original)
	if err != nil {
		t.Fatal(err)
	}

	if binary.LittleEndian.Uint32(encoded[8:]) != 3 {
		t.Fatal("missing version3 transport envelope")
	}

	document, err := lightmapdata.Decode(encoded)
	if err != nil {
		t.Fatal(err)
	}

	if !document.Transport() || document.GISamples != 64 {
		t.Fatal("lost transport quality metadata")
	}

	offset := len(
		document.Nodes,
	) + len(
		document.Triangles,
	) + len(
		document.Receivers,
	) + len(
		document.Padding,
	) + len(
		document.Frames,
	) + len(
		document.Surfaces,
	)
	for _, field := range [][]byte{document.ReceiverGIIndex, document.GIReceivers, document.GIPatches, document.GITransfers} {
		if &field[0] != &document.GPUData[offset] {
			t.Fatal("transport payload requires runtime repacking")
		}

		offset += len(field)
	}

	clear(encoded)

	if document.GISamples != 64 || binary.LittleEndian.Uint32(document.GIReceivers[4:]) != 1 {
		t.Fatal("retained caller transport payload")
	}
}

func TestTransportRejectsMalformedReferencesWeightsAndMetadata(t *testing.T) {
	t.Parallel()

	for _, name := range []string{"missing-rnm", "missing-mapping", "missing-receiver", "duplicate-mapping", "uncovered-mapping", "missing-link", "link-gap", "too-many-links", "invalid-patch", "negative-weight", "nan-weight", "excessive-energy", "negative-sky", "nan-sky", "patch-normal", "patch-reflectance", "patch-environment", "patch-sidedness", "samples-low", "samples-high"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			document := lightmapfixture.TransportDocument()
			put := func(data []byte, offset int, value float32) {
				binary.LittleEndian.PutUint32(data[offset:], math.Float32bits(value))
			}

			switch name {
			case "missing-rnm":
				document.Frames = nil
				document.Surfaces = nil
			case "missing-mapping":
				document.ReceiverGIIndex = nil
			case "missing-receiver":
				document.GIReceivers = document.GIReceivers[:32]
			case "duplicate-mapping":
				binary.LittleEndian.PutUint32(document.ReceiverGIIndex[4:], 0)
			case "uncovered-mapping":
				binary.LittleEndian.PutUint32(document.ReceiverGIIndex[8:], 0)
			case "missing-link":
				document.GITransfers = nil
			case "link-gap":
				binary.LittleEndian.PutUint32(document.GIReceivers, 1)
			case "too-many-links":
				binary.LittleEndian.PutUint32(document.GIReceivers[4:], 9)
			case "invalid-patch":
				binary.LittleEndian.PutUint32(document.GITransfers, 1)
			case "negative-weight":
				put(document.GITransfers, 4, -.1)
			case "nan-weight":
				put(document.GITransfers, 4, float32(math.NaN()))
			case "excessive-energy":
				put(document.GITransfers, 4, 3)
			case "negative-sky":
				put(document.GIReceivers, 8, -1)
			case "nan-sky":
				put(document.GIReceivers, 8, float32(math.NaN()))
			case "patch-normal":
				clear(document.GIPatches[16:28])
			case "patch-reflectance":
				put(document.GIPatches, 32, 1)
			case "patch-environment":
				put(document.GIPatches, 12, 2)
			case "patch-sidedness":
				put(document.GIPatches, 44, 2)
			case "samples-low":
				document.GISamples = 15
			case "samples-high":
				document.GISamples = 129
			}

			if _, err := lightmapdata.Encode(document); err == nil {
				t.Fatal("accepted malformed prepared transport")
			}
		})
	}
}

func TestTransportHeaderAndQualityAreChecksummed(t *testing.T) {
	t.Parallel()

	encoded, err := lightmapdata.Encode(lightmapfixture.TransportDocument())
	if err != nil {
		t.Fatal(err)
	}

	for _, offset := range []int{112, 116, 120, 124, 128, 132, 143} {
		corrupt := append([]byte(nil), encoded...)

		corrupt[offset] ^= 1
		if _, err := lightmapdata.Decode(corrupt); err == nil {
			t.Fatalf("accepted corrupt transport header byte%d", offset)
		}
	}
}
