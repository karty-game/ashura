package leveldata

const (
	// RGBDiffuse stores physical diffuse irradiance divided by pi in one map.
	RGBDiffuse = "rgb-diffuse-v1"
	// RNMFit stores the nonnegative, flat-calibrated directional fit in three maps.
	RNMFit = "rnm-fit-v1"
)

// MapCount identifies the complete active coefficient family. Inactive fixed
// capacity slots are zero, never additional downloads or duplicated RGB maps.
func (page Page) MapCount() int {
	if page.Representation == RGBDiffuse {
		return 1
	}

	return len(page.Maps)
}

func validPageRepresentation(page Page) bool {
	if IsLDRStorage(page.Storage) && page.Representation == "" {
		return false
	}

	switch page.Representation {
	case "", RNMFit:
		return true
	case RGBDiffuse:
		return page.Maps[1] == (Ref{}) && page.Maps[2] == (Ref{})
	default:
		return false
	}
}

func schemaAllowsRepresentation(schema string, chunk Chunk) bool {
	for _, page := range chunk.Pages {
		if schema == Schema6 || schema == Schema7 || schema == Schema8 {
			if page.Representation != RGBDiffuse && page.Representation != RNMFit {
				return false
			}
		} else if page.Representation != "" || page.Preview {
			return false
		}
	}

	return true
}
