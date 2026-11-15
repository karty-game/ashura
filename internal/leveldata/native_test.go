//go:build !js

package leveldata

import (
	"bytes"
	"encoding/json"
	"errors"
	"testing"
)

func TestNativeHDRManifestRequiresVersionFour(t *testing.T) {
	t.Parallel()

	document := fixtureDocument(t)
	document.Schema = Schema4
	document.Chunks[0].Pages[0].Storage = "bc-hdr"
	document.Chunks[0].Pages[0].Encoding = "linear"

	encoded, err := Encode(document)
	if err != nil {
		t.Fatal(err)
	}

	decoded, err := Decode(encoded)
	if err != nil {
		t.Fatal(err)
	}

	page := decoded.Chunks[0].Pages[0]
	if decoded.Schema != Schema4 || page.Storage != "bc-hdr" || page.Encoding != "linear" {
		t.Fatalf("native HDR profile lost during roundtrip: %+v", page)
	}

	again, err := Encode(decoded)
	if err != nil || !bytes.Equal(encoded, again) {
		t.Fatalf("native HDR roundtrip changed bytes: %v", err)
	}

	for _, schema := range []string{Schema, Schema2, Schema3} {
		t.Run(schema, func(t *testing.T) {
			t.Parallel()

			legacy := fixtureDocument(t)
			legacy.Schema = schema
			legacy.Chunks[0].Pages[0].Storage = "bc-hdr"
			legacy.Chunks[0].Pages[0].Encoding = "linear"
			assertNativeManifestRejected(t, legacy)
		})
	}
}

func TestNativeHDRStorageRequiresLinearEncoding(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name, storage, encoding string
	}{
		{name: "RGBE", storage: "bc-hdr", encoding: "rgbe8"},
		{name: "unknown storage", storage: "future-hdr", encoding: "linear"},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			document := fixtureDocument(t)
			document.Schema = Schema4
			document.Chunks[0].Pages[0].Storage = test.storage
			document.Chunks[0].Pages[0].Encoding = test.encoding
			assertNativeManifestRejected(t, document)
		})
	}
}

func TestRawStorageRemainsValidAcrossManifestVersions(t *testing.T) {
	t.Parallel()

	for _, schema := range []string{Schema, Schema2, Schema3, Schema4} {
		t.Run(schema, func(t *testing.T) {
			t.Parallel()

			document := fixtureDocument(t)
			document.Schema = schema

			encoded, err := Encode(document)
			if err != nil {
				t.Fatal("raw profile rejected", err)
			}

			if _, err = Decode(encoded); err != nil {
				t.Fatal("raw wire profile rejected", err)
			}
		})
	}
}

func assertNativeManifestRejected(t *testing.T, document Document) {
	t.Helper()

	if _, err := Encode(document); !errors.Is(err, ErrFormat) {
		t.Fatalf("invalid native HDR build manifest: got %v, want ErrFormat", err)
	}

	encoded, err := json.Marshal(document)
	if err != nil {
		t.Fatal(err)
	}

	if _, err = Decode(encoded); !errors.Is(err, ErrFormat) {
		t.Fatalf("invalid native HDR wire manifest: got %v, want ErrFormat", err)
	}
}
