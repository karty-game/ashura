package leveldata

const (
	encodingLinear = "linear"
	StorageBC1LDR  = "bc1-ldr"
	StorageBC7LDR  = "bc7-ldr"
	StorageETC2LDR = "etc2-ldr"
	EncodingSRGB   = "srgb"
)

// IsLDRStorage identifies fixed native formats, without a runtime codec choice.
func IsLDRStorage(storage string) bool {
	return storage == StorageBC1LDR || storage == StorageBC7LDR || storage == StorageETC2LDR
}

func hasLDRStorage(chunk Chunk) bool {
	for _, page := range chunk.Pages {
		if IsLDRStorage(page.Storage) || page.Encoding == EncodingSRGB {
			return true
		}
	}

	return false
}
