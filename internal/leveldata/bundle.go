package leveldata

const maxChunkPages = 64

// ChunkOwnedReferences returns the distinct geometry, UV and lightmap payloads
// owned by a chunk. Material textures and development bake inputs stay external.
func ChunkOwnedReferences(chunk Chunk) ([]Ref, error) {
	if len(chunk.Pages) > maxChunkPages {
		return nil, ErrFormat
	}

	refs := newReferences()
	result := []Ref{}
	size := int64(0)

	for _, page := range chunk.Pages {
		for _, ref := range append([]Ref{page.Geometry, page.UV}, page.Maps[:page.MapCount()]...) {
			_, found := refs.hashes[ref.SHA256]
			if err := refs.add(ref); err != nil {
				return nil, err
			}

			if found {
				continue
			}

			size += int64(ref.Size)
			if size > MaxChunkBytes {
				return nil, ErrFormat
			}

			result = append(result, ref)
		}
	}

	return result, nil
}

func validateChunkBundle(chunk Chunk, refs []Ref) error {
	bundle := chunk.Bundle

	ownedRefs, err := ChunkOwnedReferences(chunk)
	if err != nil || len(bundle.Entries) != len(ownedRefs) || len(ownedRefs) == 0 {
		return ErrFormat
	}

	owned := make(map[string]Ref, len(ownedRefs))
	for _, ref := range ownedRefs {
		owned[ref.SHA256] = ref
	}

	seen := make(map[string]bool, len(ownedRefs))
	end := int64(0)

	for _, entry := range bundle.Entries {
		ref, found := owned[entry.SHA256]
		if !found || seen[entry.SHA256] || entry.Offset < end ||
			entry.Offset > int64(bundle.File.Size)-int64(ref.Size) {
			return ErrFormat
		}

		end = entry.Offset + int64(ref.Size)
		seen[entry.SHA256] = true
	}

	return validateBundleTransport(bundle, refs, owned)
}

func validateBundleTransport(bundle *ChunkBundle, refs []Ref, owned map[string]Ref) error {
	size := int64(bundle.File.Size)

	for _, ref := range refs {
		if _, bundled := owned[ref.SHA256]; bundled || ref.SHA256 == bundle.File.SHA256 {
			continue
		}

		size += int64(ref.Size)
		if size > MaxChunkBytes {
			return ErrFormat
		}
	}

	return nil
}
