package leveldata

import "testing"

func TestLDRDeliveryRequiresVersionedTransferAndCalibratedMaps(t *testing.T) {
	t.Parallel()

	for _, storage := range []string{StorageBC1LDR, StorageBC7LDR, StorageETC2LDR} {
		for _, transfer := range []string{"linear", EncodingSRGB} {
			for _, representation := range []string{RGBDiffuse, RNMFit} {
				t.Run(storage+"/"+transfer+"/"+representation, func(t *testing.T) {
					t.Parallel()

					checkLDRDelivery(t, storage, transfer, representation)
				})
			}
		}
	}
}

func TestHardwareSRGBIsNotAnUnversionedLightmapEncoding(t *testing.T) {
	t.Parallel()

	document := fixtureDocument(t)
	document.Schema = Schema7
	page := &document.Chunks[0].Pages[0]

	page.Representation, page.Encoding = RNMFit, EncodingSRGB
	for _, storage := range []string{"", "bc-hdr", "etc2-astc", "future-ldr"} {
		page.Storage = storage
		if Validate(document) == nil {
			t.Fatal("non-LDR storage admitted hardware transfer", storage)
		}
	}
}

func checkLDRDelivery(t *testing.T, storage, transfer, representation string) {
	t.Helper()

	document := fixtureDocument(t)
	document.Schema = Schema7
	page := &document.Chunks[0].Pages[0]

	page.Storage, page.Encoding, page.Representation = storage, transfer, representation
	if representation == RGBDiffuse {
		page.Maps[1], page.Maps[2] = Ref{}, Ref{}
	}

	encoded, err := Encode(document)
	if err != nil {
		t.Fatal(err)
	}

	decoded, err := Decode(encoded)
	if err != nil || decoded.Chunks[0].Pages[0].Storage != storage || decoded.Chunks[0].Pages[0].Encoding != transfer {
		t.Fatal("LDR profile changed during admission", err)
	}

	for _, schema := range []string{Schema, Schema2, Schema3, Schema4, Schema5, Schema6} {
		document.Schema = schema
		if Validate(document) == nil {
			t.Fatal("older manifest accepted LDR storage", schema)
		}
	}

	page.Encoding = "rgbe8"

	if ValidateChunk(document.Chunks[0]) == nil {
		t.Fatal("LDR storage admitted exponent alpha")
	}

	page.Encoding, page.Representation = transfer, ""

	if ValidateChunk(document.Chunks[0]) == nil {
		t.Fatal("independent LDR chunk inferred legacy units")
	}
}
