package leveldata

import "testing"

func TestLargeAtlasRequiresVersionTwoAndRetainsBounds(t *testing.T) {
	t.Parallel()
	document := fixtureDocument(t)
	document.Chunks[0].Pages[0].Width = 2048

	document.Chunks[0].Pages[0].Height = 2048
	if _, err := Encode(document); err == nil {
		t.Fatal("v1 admitted a v2 atlas")
	}

	document.Schema = Schema2

	encoded, err := Encode(document)
	if err != nil {
		t.Fatal(err)
	}

	if _, err = Decode(encoded); err != nil {
		t.Fatal("v2 atlas rejected", err)
	}

	document.Chunks[0].Pages[0].Width = 4096
	if _, err = Encode(document); err == nil {
		t.Fatal("v2 admitted an oversized atlas")
	}
}
