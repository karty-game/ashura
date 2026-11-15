//nolint:gosec // ValidateRef proves positive payload sizes below 128 MiB before summing uint64 bytes.
package leveldata

// MaxChunkBytes bounds the total distinct prepared files fetched for one chunk.
const MaxChunkBytes = 256 << 20

type references struct {
	paths  map[string]Ref
	hashes map[string]int
}

func newReferences() references {
	return references{paths: make(map[string]Ref), hashes: make(map[string]int)}
}
func (refs references) add(ref Ref) error {
	if err := ValidateRef(ref); err != nil {
		return err
	}

	if previous, found := refs.paths[ref.Path]; found && previous != ref {
		return ErrFormat
	}

	if size, found := refs.hashes[ref.SHA256]; found && size != ref.Size {
		return ErrFormat
	}

	refs.paths[ref.Path], refs.hashes[ref.SHA256] = ref, ref.Size

	return nil
}

// ChunkReferences returns stable first-use references, deduplicated by content.
// Conflicting path/hash descriptors and oversized aggregate requests fail before I/O.
func ChunkReferences(chunk Chunk) ([]Ref, error) {
	refs := newReferences()
	result := []Ref{}
	size := uint64(0)

	for _, ref := range chunkReferences(chunk) {
		_, exists := refs.hashes[ref.SHA256]
		if err := refs.add(ref); err != nil {
			return nil, err
		}

		if exists {
			continue
		}

		size += uint64(ref.Size)
		if size > MaxChunkBytes {
			return nil, ErrFormat
		}

		result = append(result, ref)
	}

	return result, nil
}

func chunkReferences(chunk Chunk) []Ref {
	var refs []Ref
	for _, page := range chunk.Pages {
		refs = append(refs, page.Geometry, page.UV)

		refs = append(refs, page.Maps[:page.MapCount()]...)
		for _, material := range page.Materials {
			if material.Albedo != nil {
				refs = append(refs, *material.Albedo)
			}

			if material.SSBump != nil {
				refs = append(refs, *material.SSBump)
			}
		}
	}

	return refs
}

func chunkMetadataReferences(chunk Chunk) []Ref {
	refs := chunkReferences(chunk)
	for _, page := range chunk.Pages {
		if page.BakeInputs != nil {
			refs = append(refs, *page.BakeInputs)
		}
	}

	return refs
}
