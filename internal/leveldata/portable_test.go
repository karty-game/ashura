package leveldata

import "testing"

func TestPortableProfilesNeedVersionFive(t *testing.T) {
	t.Parallel()

	for _, schema := range []string{Schema, Schema2, Schema3, Schema4, Schema5} {
		t.Run(schema, func(t *testing.T) {
			t.Parallel()
			document := fixtureDocument(t)
			document.Schema = schema
			page := &document.Chunks[0].Pages[0]
			page.Storage = "etc2-astc"
			page.Encoding = "rgbe8"
			page.Materials[0].AlbedoEncoding = "etc2"

			data, err := Encode(document)
			if schema != Schema5 {
				if err == nil {
					t.Fatal("legacy manifest accepted mobile storage")
				}

				return
			}

			if err != nil {
				t.Fatal(err)
			}

			decoded, err := Decode(data)
			if err != nil {
				t.Fatal(err)
			}

			if decoded.Chunks[0].Pages[0].Storage != "etc2-astc" {
				t.Fatal("mobile profile lost")
			}

			document.Chunks[0].Pages[0].Encoding = "linear"
			if _, err := Encode(document); err == nil {
				t.Fatal("mobile profile accepted linear HDR metadata")
			}
		})
	}
}

func TestNativeAlbedoEncodingsAreVersioned(t *testing.T) {
	t.Parallel()

	for _, encoding := range []string{"bc1", "etc2"} {
		t.Run(encoding, func(t *testing.T) {
			t.Parallel()
			document := fixtureDocument(t)
			document.Schema = Schema5

			document.Chunks[0].Pages[0].Materials[0].AlbedoEncoding = encoding
			if _, err := Encode(document); err != nil {
				t.Fatal(err)
			}

			document.Schema = Schema4
			if _, err := Encode(document); err == nil {
				t.Fatal("legacy schema accepted native albedo encoding")
			}
		})
	}
}
