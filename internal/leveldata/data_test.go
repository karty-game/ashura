package leveldata

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"math"
	"strings"
	"testing"
)

func fixtureRef(t *testing.T) Ref {
	t.Helper()

	ref, err := Reference("payload/one.bin", []byte{1})
	if err != nil {
		t.Fatal(err)
	}

	return ref
}
func fixtureDocument(t *testing.T) Document {
	t.Helper()
	ref := fixtureRef(t)

	return Document{Schema: Schema, Source: "levels/test.scene.yaml", ChunkSize: [3]float32{32, 32, 32}, TexelsPerUnit: 16,
		Chunks: []Chunk{{ID: "default/0/0/0", Space: "default", Min: [3]float32{}, Max: [3]float32{32, 32, 32}, Sources: []string{"/mesh"},
			Pages: []Page{{Geometry: ref, UV: ref, Maps: [3]Ref{ref, ref, ref}, Scale: 1, Encoding: "rgbe8", Width: 16, Height: 16,
				Materials: []Material{{Name: "batch_0", Color: [4]float32{1, 1, 1, 1}, Albedo: &ref}}}}}},
		Objects: []Object{{ID: "/mesh", Model: "model", Transform: [16]float32{1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1}}}}
}

func TestManifestRoundTripAndCompleteFamilyAdmission(t *testing.T) {
	t.Parallel()
	document := fixtureDocument(t)

	encoded, err := Encode(document)
	if err != nil {
		t.Fatal(err)
	}

	decoded, err := Decode(encoded)
	if err != nil {
		t.Fatal(err)
	}

	again, err := Encode(decoded)
	if err != nil || !bytes.Equal(again, encoded) {
		t.Fatalf("roundtrip: %v", err)
	}

	cases := map[string]func(*Document){
		"wrong schema":            func(doc *Document) { doc.Schema = "future@2" },
		"duplicate cell":          func(doc *Document) { next := doc.Chunks[0]; next.ID = "another"; doc.Chunks = append(doc.Chunks, next) },
		"duplicate object":        func(doc *Document) { doc.Objects = append(doc.Objects, doc.Objects[0]) },
		"unknown source":          func(doc *Document) { doc.Chunks[0].Sources = []string{"/unknown"} },
		"bad transform":           func(doc *Document) { doc.Objects[0].Transform[15] = 0 },
		"dynamic without asset":   func(doc *Document) { doc.Objects[0].Dynamic = true },
		"invalid encoding":        func(doc *Document) { doc.Chunks[0].Pages[0].Encoding = "srgb" },
		"bad width":               func(doc *Document) { doc.Chunks[0].Pages[0].Width = 17 },
		"nonfinite":               func(doc *Document) { doc.Chunks[0].Pages[0].Scale = float32(math.Inf(1)) },
		"inverted bounds":         func(doc *Document) { doc.Chunks[0].Min[0] = 64 },
		"outside origin":          func(doc *Document) { doc.Chunks[0].Origin[0] = -1 },
		"oversized bounds":        func(doc *Document) { doc.Chunks[0].Max[0] = 64 },
		"conflicting path":        func(doc *Document) { doc.Chunks[0].Pages[0].UV.SHA256 = strings.Repeat("f", 64) },
		"conflicting digest size": func(doc *Document) { doc.Chunks[0].Pages[0].UV.Path = "other.bin"; doc.Chunks[0].Pages[0].UV.Size = 2 },
	}
	for name, change := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			doc := fixtureDocument(t)
			change(&doc)

			if err := Validate(doc); err == nil {
				t.Fatal("malformed family admitted")
			}
		})
	}

	duplicate := append([]byte(`{"schema":"karty.baked-level@1",`), encoded[1:]...)
	if _, err = Decode(duplicate); err == nil {
		t.Fatal("duplicate JSON key admitted")
	}

	for _, changed := range []string{
		strings.Replace(string(encoded), `"schema":`, `"Schema":`, 1),
		strings.Replace(string(encoded), `"chunkSize":[32,32,32]`, `"chunkSize":[32,32,32,1]`, 1),
		strings.Replace(string(encoded), `"color":[1,1,1,1]`, `"color":[1,1,1,1,1]`, 1),
	} {
		if _, err = Decode([]byte(changed)); err == nil {
			t.Fatal("noncanonical key or oversized fixed array admitted")
		}
	}

	unknown := append(append([]byte{}, encoded[:len(encoded)-1]...), []byte(`,"unknown":true}`)...)
	if _, err = Decode(unknown); err == nil {
		t.Fatal("unknown JSON property admitted")
	}

	if _, err = Decode(append(encoded, []byte(" {}")...)); err == nil {
		t.Fatal("trailing document admitted")
	}
}

func TestReferencesRejectEscapesAndBoundCompleteFetch(t *testing.T) {
	t.Parallel()

	ref := fixtureRef(t)
	for _, path := range []string{".", "../outside", "/absolute", "bad\\path", "bad%2fpath", "scheme:thing", "bad?query", "bad#fragment", "bad\nname"} {
		value := ref

		value.Path = path
		if err := ValidateRef(value); err == nil {
			t.Fatalf("unsafe path %q admitted", path)
		}
	}

	chunk := fixtureDocument(t).Chunks[0]
	for index := range 3 {
		chunk.Pages[0].Maps[index] = Ref{
			Path:   string(rune('a'+index)) + ".bin",
			Size:   MaxPayloadBytes,
			SHA256: strings.Repeat(string(rune('a'+index)), 64),
		}
	}

	if _, err := ChunkReferences(chunk); err == nil {
		t.Fatal("unbounded chunk request admitted")
	}
}

func TestDeveloperBakeReferenceIsAdmittedButExcludedFromProductionRequests(t *testing.T) {
	t.Parallel()

	document := fixtureDocument(t)
	developer := Ref{Path: "development/bake.klightmap", Size: MaxPayloadBytes, SHA256: strings.Repeat("a", 64)}
	document.Chunks[0].Pages[0].BakeInputs = &developer

	encoded, err := Encode(document)
	if err != nil {
		t.Fatal(err)
	}

	decoded, err := Decode(encoded)
	if err != nil || decoded.Chunks[0].Pages[0].BakeInputs == nil || *decoded.Chunks[0].Pages[0].BakeInputs != developer {
		t.Fatalf("developer input reference lost: %v", err)
	}

	refs, err := ChunkReferences(document.Chunks[0])
	if err != nil || len(refs) != 1 || refs[0].SHA256 == developer.SHA256 {
		t.Fatalf("production fetch includes developer visibility scene: %v, %v", refs, err)
	}

	developer.Path = "../escape"

	if err = Validate(document); err == nil {
		t.Fatal("unsafe developer input reference admitted")
	}
}

func TestUVLayoutChecksumBindingOwnershipAndRanges(t *testing.T) {
	t.Parallel()

	digest := sha256.Sum256([]byte("prepared geometry"))
	stream := make([]byte, 24)
	binary.LittleEndian.PutUint32(stream[8:], math.Float32bits(.5))
	binary.LittleEndian.PutUint32(stream[16:], math.Float32bits(1))

	encoded, err := EncodeUV(digest, [][]byte{stream})
	if err != nil {
		t.Fatal(err)
	}

	layout, err := DecodeUV(encoded)
	if err != nil || layout.ModelDigest() != digest || layout.PartCount() != 1 || !bytes.Equal(layout.PartUV(0), stream) {
		t.Fatalf("layout roundtrip: %v", err)
	}

	binary.LittleEndian.PutUint32(encoded[uvHeader+4:], math.Float32bits(.25))

	if binary.LittleEndian.Uint32(layout.PartUV(0)) != 0 {
		t.Fatal("decoded layout aliases caller bytes")
	}

	if layout.PartUV(-1) != nil || layout.PartUV(1) != nil {
		t.Fatal("out-of-range UV stream returned")
	}

	encoded, err = EncodeUV(digest, [][]byte{stream})
	if err != nil {
		t.Fatal(err)
	}

	for length := range encoded {
		if _, err = DecodeUV(encoded[:length]); err == nil {
			t.Fatalf("truncation %d admitted", length)
		}
	}

	changes := map[string]func([]byte){
		"count":   func(data []byte) { binary.LittleEndian.PutUint32(data[12:], math.MaxUint32) },
		"version": func(data []byte) { binary.LittleEndian.PutUint32(data[8:], 2) },
		"length":  func(data []byte) { binary.LittleEndian.PutUint32(data[uvHeader:], math.MaxUint32) },
		"nan": func(data []byte) {
			binary.LittleEndian.PutUint32(data[uvHeader+4:], math.Float32bits(float32(math.NaN())))
		},
		"outside UV": func(data []byte) { binary.LittleEndian.PutUint32(data[uvHeader+4:], math.Float32bits(1.1)) },
	}
	for name, change := range changes {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			data := bytes.Clone(encoded)
			change(data)
			checksum := uvChecksum(data)
			copy(data[48:80], checksum[:])

			if _, err := DecodeUV(data); err == nil {
				t.Fatal("malformed layout admitted")
			}
		})
	}
}

func FuzzDecodeManifest(f *testing.F) {
	document := Document{Schema: Schema, Source: "fixture", ChunkSize: [3]float32{32, 32, 32}, TexelsPerUnit: 16}

	seed, err := json.Marshal(document)
	if err != nil {
		f.Fatal(err)
	}

	f.Add(seed)
	f.Add([]byte(`{"schema":"first","schema":"second"}`))
	f.Fuzz(func(t *testing.T, data []byte) {
		doc, err := Decode(data)
		if err == nil {
			if _, err = Encode(doc); err != nil {
				t.Fatal(err)
			}
		}
	})
}

func FuzzDecodeUV(f *testing.F) {
	encoded, _ := EncodeUV(sha256.Sum256([]byte("fixture")), [][]byte{make([]byte, 24)})
	f.Add(encoded)
	f.Add([]byte("KARTUV1"))
	f.Fuzz(func(t *testing.T, data []byte) {
		layout, err := DecodeUV(data)
		if err == nil {
			if layout.PartCount() < 1 || layout.PartUV(-1) != nil || layout.PartUV(layout.PartCount()) != nil {
				t.Fatal("invalid admitted layout")
			}
		}
	})
}
