package leveldata

import "testing"

func TestCalibratedRepresentationsAreVersionedAndComplete(t *testing.T) {
	t.Parallel()

	for _, representation := range []string{RGBDiffuse, RNMFit} {
		t.Run(representation, func(t *testing.T) {
			t.Parallel()
			document := fixtureDocument(t)
			document.Schema = Schema6
			page := &document.Chunks[0].Pages[0]

			page.Representation = representation

			page.Preview = true
			if representation == RGBDiffuse {
				page.Maps[1], page.Maps[2] = Ref{}, Ref{}
			}

			encoded, err := Encode(document)
			if err != nil {
				t.Fatal(err)
			}

			decoded, err := Decode(encoded)
			if err != nil || decoded.Chunks[0].Pages[0].MapCount() != page.MapCount() {
				t.Fatal(decoded, err)
			}

			for _, schema := range []string{Schema, Schema2, Schema3, Schema4, Schema5} {
				document.Schema = schema
				if Validate(document) == nil {
					t.Fatal("older schema accepted new units", schema)
				}
			}

			document.Schema = Schema6

			if representation == RGBDiffuse {
				page.Maps[1] = page.Maps[0]

				if Validate(document) == nil {
					t.Fatal("inactive payload admitted")
				}
			}
		})
	}

	document := fixtureDocument(t)

	document.Schema = Schema6
	if Validate(document) == nil {
		t.Fatal("new schema silently inferred legacy units")
	}
}
