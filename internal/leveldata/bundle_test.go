package leveldata

import (
	"errors"
	"math"
	"reflect"
	"testing"
)

func bundleFixture(t *testing.T) Document {
	t.Helper()
	document := fixtureDocument(t)
	document.Schema = Schema8
	page := &document.Chunks[0].Pages[0]
	page.Representation = RNMFit

	reference := func(name string) Ref {
		t.Helper()

		ref, err := Reference(name+".bin", []byte(name))
		if err != nil {
			t.Fatal(err)
		}

		return ref
	}
	page.Geometry, page.UV = reference("geometry"), reference("uv")
	page.Maps = [3]Ref{reference("light0"), reference("light1"), reference("light2")}
	albedo, development := reference("material"), reference("development")
	page.Materials[0].Albedo = &albedo
	page.BakeInputs = &development

	owned, err := ChunkOwnedReferences(document.Chunks[0])
	if err != nil {
		t.Fatal(err)
	}

	file, err := Reference("chunks/one.zip", make([]byte, 8192))
	if err != nil {
		t.Fatal(err)
	}

	bundle := &ChunkBundle{File: file}
	offset := int64(40)

	for _, ref := range owned {
		bundle.Entries = append(bundle.Entries, BundleEntry{SHA256: ref.SHA256, Offset: offset})
		offset += int64(ref.Size) + 40
	}

	document.Chunks[0].Bundle = bundle

	return document
}

func TestChunkBundleManifestRoundTripAndOwnedReferences(t *testing.T) {
	t.Parallel()
	document := bundleFixture(t)
	chunk := &document.Chunks[0]
	chunk.Pages = append(chunk.Pages, chunk.Pages[0])

	owned, err := ChunkOwnedReferences(*chunk)
	if err != nil || len(owned) != 5 {
		t.Fatal("owned family did not deduplicate pages", owned, err)
	}

	for _, ref := range owned {
		if ref == *chunk.Pages[0].Materials[0].Albedo || ref == *chunk.Pages[0].BakeInputs {
			t.Fatal("material/development reference entered chunk bundle")
		}
	}

	logical, err := ChunkReferences(*chunk)
	if err != nil || len(logical) != 6 {
		t.Fatal("bundle changed logical residency references", logical, err)
	}

	encoded, err := Encode(document)
	if err != nil {
		t.Fatal(err)
	}

	actual, err := Decode(encoded)
	if err != nil || !reflect.DeepEqual(actual, document) {
		t.Fatal("bundle roundtrip differs", actual, err)
	}

	document.Chunks[0].Bundle = nil
	if err := Validate(document); err != nil {
		t.Fatal("schema8 must also admit independent chunks", err)
	}
}

func TestChunkBundlesRejectUnboundOrOverlappingEntries(t *testing.T) {
	t.Parallel()

	cases := map[string]func(*Document){
		"old schema":    func(doc *Document) { doc.Schema = Schema7 },
		"missing entry": func(doc *Document) { doc.Chunks[0].Bundle.Entries = doc.Chunks[0].Bundle.Entries[:4] },
		"duplicate entry": func(doc *Document) {
			bundle := doc.Chunks[0].Bundle
			bundle.Entries[1].SHA256 = bundle.Entries[0].SHA256
		},
		"material in bundle": func(doc *Document) {
			doc.Chunks[0].Bundle.Entries[0].SHA256 = doc.Chunks[0].Pages[0].Materials[0].Albedo.SHA256
		},
		"overlap": func(doc *Document) {
			bundle := doc.Chunks[0].Bundle
			bundle.Entries[1].Offset = bundle.Entries[0].Offset + 1
		},
		"unsorted": func(doc *Document) {
			entries := doc.Chunks[0].Bundle.Entries
			entries[0], entries[1] = entries[1], entries[0]
		},
		"negative offset": func(doc *Document) { doc.Chunks[0].Bundle.Entries[0].Offset = -1 },
		"overflow offset": func(doc *Document) { doc.Chunks[0].Bundle.Entries[0].Offset = math.MaxInt64 },
		"beyond end": func(doc *Document) {
			doc.Chunks[0].Bundle.Entries[4].Offset = int64(doc.Chunks[0].Bundle.File.Size)
		},
		"oversized file": func(doc *Document) { doc.Chunks[0].Bundle.File.Size = MaxPayloadBytes + 1 },
		"unsafe file":    func(doc *Document) { doc.Chunks[0].Bundle.File.Path = "../outside.zip" },
		"conflicting path": func(doc *Document) {
			doc.Chunks[0].Bundle.File.Path = doc.Chunks[0].Pages[0].Geometry.Path
		},
	}
	for name, change := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			document := bundleFixture(t)
			change(&document)

			if err := Validate(document); !errors.Is(err, ErrFormat) {
				t.Fatal("invalid bundle admitted", err)
			}
		})
	}
}

func TestChunkBundleTransportBoundIncludesArchiveOverhead(t *testing.T) {
	t.Parallel()
	document := bundleFixture(t)
	chunk := &document.Chunks[0]
	chunk.Bundle.File.Size = MaxPayloadBytes
	chunk.Pages[0].Materials[0].Albedo.Size = MaxPayloadBytes

	if err := Validate(document); err != nil {
		t.Fatal("exact transport bound rejected", err)
	}

	additional := fixtureRef(t)
	chunk.Pages[0].Materials[0].SSBump = &additional

	if err := Validate(document); !errors.Is(err, ErrFormat) {
		t.Fatal("archive plus shared resources exceeded transport bound", err)
	}

	chunk.Bundle.File.Size--

	if err := Validate(document); err != nil {
		t.Fatal("transport payloads at exact bound rejected", err)
	}
}
