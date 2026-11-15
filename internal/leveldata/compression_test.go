package leveldata

import (
	"encoding/json"
	"testing"
)

func TestETC1SRequiresVersionThreeAndExplicitRole(t *testing.T) {
	t.Parallel()
	document := fixtureDocument(t)
	mat := &document.Chunks[0].Pages[0].Materials[0]
	ref := document.Chunks[0].Pages[0].Maps[0]
	mat.Albedo = &ref
	mat.AlbedoEncoding = "etc1s"

	for _, schema := range []string{Schema, Schema2} {
		document.Schema = schema
		if _, err := Encode(document); err == nil {
			t.Fatalf("legacy %s admitted ETC1S", schema)
		}
	}

	document.Schema = Schema3

	encoded, err := Encode(document)
	if err != nil {
		t.Fatal(err)
	}

	if _, err = Decode(encoded); err != nil {
		t.Fatal(err)
	}

	mat.AlbedoEncoding = "jpeg"

	if _, err = Encode(document); err == nil {
		t.Fatal("unknown encoding admitted")
	}

	mat.AlbedoEncoding = "etc1s"
	mat.Albedo = nil

	if _, err = Encode(document); err == nil {
		t.Fatal("encoding without texture admitted")
	}
}

func TestConflictingAlbedoEncodingRejectedBeforePayloadIO(t *testing.T) {
	t.Parallel()

	for _, scope := range []string{"material", "page", "chunk"} {
		for _, firstEncoding := range []string{"", "etc1s"} {
			t.Run(scope+"/"+firstEncoding, func(t *testing.T) {
				t.Parallel()

				document := conflictingAlbedoDocument(t, scope, firstEncoding)
				assertConflictingAlbedoAdmission(t, document, scope)
				setAlbedoEncoding(&document, firstEncoding)

				if _, err := Encode(document); err != nil {
					t.Fatal("consistent shared albedo rejected", err)
				}
			})
		}
	}
}

func assertConflictingAlbedoAdmission(t *testing.T, document Document, scope string) {
	t.Helper()

	if err := Validate(document); err == nil {
		t.Fatal("complete manifest admitted conflicting interpretations of one albedo")
	}

	encoded, err := json.Marshal(document)
	if err != nil {
		t.Fatal(err)
	}

	if _, err = Decode(encoded); err == nil {
		t.Fatal("wire manifest admitted conflicting albedo encodings")
	}

	for _, chunk := range document.Chunks {
		err = ValidateChunk(chunk)
		if scope == "chunk" && err != nil {
			t.Fatal("individually consistent chunk rejected", err)
		}

		if scope != "chunk" && err == nil {
			t.Fatal("standalone chunk admitted conflicting albedo encodings")
		}
	}
}

func setAlbedoEncoding(document *Document, encoding string) {
	for i := range document.Chunks {
		for j := range document.Chunks[i].Pages {
			for k := range document.Chunks[i].Pages[j].Materials {
				document.Chunks[i].Pages[j].Materials[k].AlbedoEncoding = encoding
			}
		}
	}
}

func conflictingAlbedoDocument(t *testing.T, scope, firstEncoding string) Document {
	t.Helper()

	document := fixtureDocument(t)
	document.Schema = Schema3
	page := document.Chunks[0].Pages[0]
	first, second := page.Materials[0], page.Materials[0]
	first.Name, second.Name = "first", "second"
	first.AlbedoEncoding = firstEncoding

	second.AlbedoEncoding = "etc1s"
	if firstEncoding == "etc1s" {
		second.AlbedoEncoding = ""
	}

	document.Chunks[0].Pages[0].Materials = []Material{first}
	page.Materials = []Material{second}

	switch scope {
	case "material":
		document.Chunks[0].Pages[0].Materials = []Material{first, second}
	case "page":
		document.Chunks[0].Pages = append(document.Chunks[0].Pages, page)
	case "chunk":
		chunk := document.Chunks[0]
		chunk.ID = "default/1/0/0"
		chunk.Cell[0] = 1
		chunk.Origin[0], chunk.Min[0], chunk.Max[0] = 32, 32, 64
		chunk.Pages = []Page{page}
		document.Chunks = append(document.Chunks, chunk)
	default:
		t.Fatal("unknown fixture scope", scope)
	}

	return document
}
