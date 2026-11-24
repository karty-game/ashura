package level

import "github.com/karty-game/ashura/pkg/scene/resource"

type bundleRead struct {
	file       Reference
	entries    []resource.Slice
	retainFile bool
}

// readTargets replaces only chunk-owned dependencies with their prepared
// archive. Shared material textures remain independent residency-aware reads.
func (prepared *PreparedChunk) readTargets(missing []Reference) ([]Reference, bundleRead) {
	bundle := prepared.chunk.Bundle
	if bundle == nil {
		return missing, bundleRead{}
	}

	byDigest := make(map[string]Reference, len(missing))
	for _, ref := range missing {
		byDigest[ref.SHA256] = ref
	}

	var entries []resource.Slice

	for _, entry := range bundle.Entries {
		if ref, found := byDigest[entry.SHA256]; found {
			delete(byDigest, entry.SHA256)
			// Chunk admission bounds each offset by the 128 MiB file limit.
			entries = append(entries, resource.Slice{Asset: prepared.resourceAsset(ref), Offset: int(entry.Offset)})
		}
	}

	if len(entries) == 0 {
		return missing, bundleRead{}
	}

	// Chunk admission already counted this archive and all external references.
	// Residency can only reduce that admitted transport set.
	targets := []Reference{bundle.File}
	for _, ref := range missing {
		if _, loose := byDigest[ref.SHA256]; loose && ref.SHA256 != bundle.File.SHA256 {
			targets = append(targets, ref)
		}
	}

	_, retainFile := byDigest[bundle.File.SHA256]

	return targets, bundleRead{file: bundle.File, entries: entries, retainFile: retainFile}
}

func (prepared *PreparedChunk) resourceAsset(ref Reference) resource.Asset {
	return resource.Asset{ID: prepared.chunk.ID, Path: ref.Path, Size: ref.Size, SHA256: ref.SHA256}
}
