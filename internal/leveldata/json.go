//nolint:mnd // Wire v1 fixes vector/matrix arities and the JSON nesting admission bound.
package leveldata

import (
	"bytes"
	"encoding/json"
	"io"
)

// Reject duplicate object keys before decoding typed records. This admission
// runs once on the bounded manifest, never during rendering or camera updates.
func uniqueJSONKeys(data []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()

	if err := scanJSONValue(decoder, 0, 0); err != nil {
		return err
	}

	if _, err := decoder.Token(); err != io.EOF {
		return ErrFormat
	}

	return nil
}
func scanJSONValue(decoder *json.Decoder, depth, expected int) error {
	if depth > 64 {
		return ErrFormat
	}

	token, err := decoder.Token()
	if err != nil {
		return ErrFormat
	}

	delimiter, isDelimiter := token.(json.Delim)
	if expected > 0 && (!isDelimiter || delimiter != '[') {
		return ErrFormat
	}

	if !isDelimiter {
		return nil
	}

	switch delimiter {
	case '{':
		return scanJSONObject(decoder, depth)
	case '[':
		return scanJSONArray(decoder, depth, expected)
	default:
		return ErrFormat
	}
}

func scanJSONObject(decoder *json.Decoder, depth int) error {
	seen := make(map[string]bool)

	for decoder.More() {
		key, err := decoder.Token()

		name, ok := key.(string)
		if err != nil || !ok || seen[name] || !canonicalJSONKey(name) {
			return ErrFormat
		}

		seen[name] = true
		if err = scanJSONValue(decoder, depth+1, arrayLength(name)); err != nil {
			return err
		}
	}

	token, err := decoder.Token()
	if err != nil || token != json.Delim('}') {
		return ErrFormat
	}

	return nil
}

func scanJSONArray(decoder *json.Decoder, depth, expected int) error {
	count := 0
	for decoder.More() {
		count++
		if expected > 0 && count > expected {
			return ErrFormat
		}

		if err := scanJSONValue(decoder, depth+1, 0); err != nil {
			return err
		}
	}

	if expected > 0 && count != expected {
		return ErrFormat
	}

	token, err := decoder.Token()
	if err != nil || token != json.Delim(']') {
		return ErrFormat
	}

	return nil
}

func arrayLength(name string) int {
	switch name {
	case "chunkSize", "cell", "origin", "min", "max", "maps":
		return 3
	case "color":
		return 4
	case "transform":
		return 16
	default:
		return 0
	}
}

// encoding/json accepts case-insensitive field aliases. Wire v1 requires its
// exact property spelling so differently cased aliases cannot bypass duplicate
// detection. Typed decoding below separately enforces each object's field set.
func canonicalJSONKey(name string) bool {
	switch name {
	case "schema", "source", "chunkSize", "texelsPerUnit", "chunks", "objects",
		"id", "space", "cell", "origin", "min", "max", "sources", "pages",
		"geometry", "uv", "materials", "maps", "scale", "encoding", "representation", "preview", "storage", "width", "height",
		"name", "color", "doubleSided", "albedo", "albedoEncoding", "ssbump", "path", "size", "sha256",
		"saveId", "model", "isDynamic", "transform", "asset", "bakeInputs", "bundle", "file", "entries", "offset":
		return true
	default:
		return false
	}
}
