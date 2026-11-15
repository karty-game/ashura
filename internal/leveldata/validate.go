package leveldata

import "math"

// Validate proves page families, logical IDs and all external references before publication/loading.
func Validate(doc Document) error {
	if doc.Schema != Schema && doc.Schema != Schema2 && doc.Schema != Schema3 && doc.Schema != Schema4 && doc.Schema != Schema5 &&
		doc.Schema != Schema6 && doc.Schema != Schema7 ||
		!validText(doc.Source) ||
		len(doc.Chunks) > MaxChunks ||
		len(doc.Objects) > 8192 ||
		!finite(doc.TexelsPerUnit) ||
		doc.TexelsPerUnit <= 0 {
		return ErrFormat
	}

	for _, value := range doc.ChunkSize {
		if !finite(value) || value <= 0 {
			return ErrFormat
		}
	}

	refs := newReferences()
	if err := validateChunks(doc, refs); err != nil {
		return err
	}

	return validateObjects(doc, refs)
}

const chunkExtentTolerance = 1e-5

func validateChunks(doc Document, refs references) error {
	maximum := uint32(1024)
	if doc.Schema != Schema {
		maximum = 2048
	}

	ids := make(map[string]bool)
	cells := make(map[struct {
		space string
		cell  [3]int32
	}]bool)
	pages := 0
	encodings := make(map[string]string)

	for _, chunk := range doc.Chunks {
		if !schemaAllowsChunkProfiles(doc.Schema, chunk) {
			return ErrFormat
		}

		key := struct {
			space string
			cell  [3]int32
		}{chunk.Space, chunk.Cell}
		if ids[chunk.ID] || cells[key] {
			return ErrFormat
		}

		ids[chunk.ID], cells[key] = true, true

		pages += len(chunk.Pages)
		if pages > MaxPages {
			return ErrFormat
		}

		if err := validateChunk(chunk, maximum, encodings); err != nil {
			return err
		}

		for axis := range 3 {
			extent := float64(chunk.Max[axis]) - float64(chunk.Min[axis])
			if extent > float64(doc.ChunkSize[axis])*(1+chunkExtentTolerance) {
				return ErrFormat
			}
		}

		for _, ref := range chunkMetadataReferences(chunk) {
			if err := refs.add(ref); err != nil {
				return err
			}
		}
	}

	return nil
}

func validateObjects(doc Document, refs references) error {
	ids := make(map[string]bool)
	saves := make(map[string]bool)
	static := make(map[string]bool)

	for _, object := range doc.Objects {
		if !validText(object.ID) || ids[object.ID] || object.Model != "" && !validText(object.Model) || !validTransform(object.Transform) {
			return ErrFormat
		}

		ids[object.ID] = true

		static[object.ID] = !object.Dynamic
		if object.SaveID != "" {
			if !validText(object.SaveID) || saves[object.SaveID] {
				return ErrFormat
			}

			saves[object.SaveID] = true
		}

		if object.Dynamic && (object.Model == "" || object.Asset == nil) {
			return ErrFormat
		}

		if object.Asset != nil {
			if err := refs.add(*object.Asset); err != nil {
				return err
			}
		}
	}

	return validateOwnership(doc, static)
}

func validateOwnership(doc Document, static map[string]bool) error {
	if len(doc.Objects) > 0 {
		for _, chunk := range doc.Chunks {
			for _, source := range chunk.Sources {
				if !static[source] {
					return ErrFormat
				}
			}
		}
	}

	return nil
}

// ValidateChunk admits one independently deliverable chunk and its complete reference family.
func ValidateChunk(chunk Chunk) error {
	return validateChunk(chunk, 2048, make(map[string]string))
}

func validateChunk(chunk Chunk, maximum uint32, encodings map[string]string) error {
	if !validText(chunk.ID) || !validText(chunk.Space) || len(chunk.Pages) == 0 || len(chunk.Pages) > 64 || len(chunk.Sources) > 8192 {
		return ErrFormat
	}

	for axis := range 3 {
		if !finite(chunk.Origin[axis]) || !finite(chunk.Min[axis]) || !finite(chunk.Max[axis]) ||
			chunk.Min[axis] >= chunk.Max[axis] || chunk.Origin[axis] < chunk.Min[axis] || chunk.Origin[axis] > chunk.Max[axis] {
			return ErrFormat
		}
	}

	sources := make(map[string]bool)
	for _, source := range chunk.Sources {
		if !validText(source) || sources[source] {
			return ErrFormat
		}

		sources[source] = true
	}

	for _, page := range chunk.Pages {
		if err := validatePage(page, maximum, encodings); err != nil {
			return err
		}
	}

	roster := newReferences()
	for _, ref := range chunkMetadataReferences(chunk) {
		if err := roster.add(ref); err != nil {
			return err
		}
	}

	_, err := ChunkReferences(chunk)

	return err
}

func validatePage(page Page, maximum uint32, encodings map[string]string) error {
	if !validPageRepresentation(page) || !validStorageEncoding(page.Storage, page.Encoding) {
		return ErrFormat
	}

	if len(page.Materials) == 0 || len(page.Materials) > 4096 || !validPageSize(page.Width, page.Height, maximum) ||
		!finite(page.Scale) || page.Scale <= 0 {
		return ErrFormat
	}

	for _, ref := range append([]Ref{page.Geometry, page.UV}, page.Maps[:page.MapCount()]...) {
		if err := ValidateRef(ref); err != nil {
			return err
		}
	}

	if page.BakeInputs != nil {
		if err := ValidateRef(*page.BakeInputs); err != nil {
			return err
		}
	}

	for _, material := range page.Materials {
		if err := validateMaterial(material); err != nil {
			return err
		}

		if err := recordAlbedoEncoding(material, encodings); err != nil {
			return err
		}
	}

	return nil
}

func hasNativeStorage(chunk Chunk) bool {
	for _, page := range chunk.Pages {
		if page.Storage != "" {
			return true
		}
	}

	return false
}

// Shared payload identities must have one interpretation across the complete
// admitted family. Reject contradictory metadata before fetching or transcoding.
func recordAlbedoEncoding(material Material, encodings map[string]string) error {
	if material.Albedo == nil {
		return nil
	}

	key := material.Albedo.SHA256
	if previous, found := encodings[key]; found && previous != material.AlbedoEncoding {
		return ErrFormat
	}

	encodings[key] = material.AlbedoEncoding

	return nil
}

func validPageSize(width, height, maximum uint32) bool {
	return width >= 16 && width <= maximum && height >= 16 && height <= maximum && width&(width-1) == 0 && height&(height-1) == 0
}

func validateMaterial(material Material) error {
	if !validAlbedoEncoding(material.AlbedoEncoding) || material.AlbedoEncoding != "" && material.Albedo == nil {
		return ErrFormat
	}

	if !validText(material.Name) || material.Color[3] != 1 {
		return ErrFormat
	}

	for _, value := range material.Color {
		if !finite(value) || value < 0 || value > 1 {
			return ErrFormat
		}
	}

	for _, ref := range []*Ref{material.Albedo, material.SSBump} {
		if ref != nil {
			if err := ValidateRef(*ref); err != nil {
				return err
			}
		}
	}

	return nil
}

func validTransform(value [16]float32) bool {
	for _, number := range value {
		if !finite(number) {
			return false
		}
	}

	if value[3] != 0 || value[7] != 0 || value[11] != 0 || value[15] != 1 {
		return false
	}

	a, b, c := float64(value[0]), float64(value[4]), float64(value[8])
	d, e, f := float64(value[1]), float64(value[5]), float64(value[9])
	g, h, i := float64(value[2]), float64(value[6]), float64(value[10])
	determinant := a*(e*i-f*h) - b*(d*i-f*g) + c*(d*h-e*g)

	return !math.IsNaN(determinant) && !math.IsInf(determinant, 0) && determinant > 0
}

func hasEncodedAlbedo(chunk Chunk) bool {
	for _, page := range chunk.Pages {
		for _, material := range page.Materials {
			if material.AlbedoEncoding != "" {
				return true
			}
		}
	}

	return false
}

func validStorageEncoding(storage, encoding string) bool {
	switch storage {
	case "":
		return encoding == encodingLinear || encoding == "rgbe8"
	case "bc-hdr":
		return encoding == encodingLinear
	case "etc2-astc":
		return encoding == "rgbe8"
	case StorageBC1LDR, StorageBC7LDR, StorageETC2LDR:
		return encoding == encodingLinear || encoding == EncodingSRGB
	default:
		return false
	}
}
func validAlbedoEncoding(encoding string) bool {
	switch encoding {
	case "", "etc1s", "bc1", "etc2":
		return true
	default:
		return false
	}
}
func hasPortableStorage(chunk Chunk) bool {
	for _, page := range chunk.Pages {
		if page.Storage == "etc2-astc" {
			return true
		}

		for _, mat := range page.Materials {
			if mat.AlbedoEncoding == "bc1" || mat.AlbedoEncoding == "etc2" {
				return true
			}
		}
	}

	return false
}

func schemaAllowsChunkProfiles(schema string, chunk Chunk) bool {
	if schema != Schema7 && hasLDRStorage(chunk) {
		return false
	}

	if schema != Schema3 && schema != Schema4 && schema != Schema5 && schema != Schema6 && schema != Schema7 && hasEncodedAlbedo(chunk) {
		return false
	}

	if schema != Schema4 && schema != Schema5 && schema != Schema6 && schema != Schema7 && hasNativeStorage(chunk) {
		return false
	}

	return schemaAllowsRepresentation(schema, chunk) &&
		(schema == Schema5 || schema == Schema6 || schema == Schema7 || !hasPortableStorage(chunk))
}
