//go:build !js

package modelbuild

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
)

const (
	maxSourceBytes = 64 << 20
	maxJSONBytes   = 8 << 20
	maxJSONDepth   = 128
	maxObjects     = 4096
	glbHeader      = 12
	chunkHeader    = 8
	jsonChunk      = 0x4e4f534a
	binChunk       = 0x004e4942
	wordSize       = 4
	floatComponent = 5126
	unsignedByte   = 5121
	unsignedShort  = 5123
	unsignedInt    = 5125
	triangles      = 4
)

var (
	errFormat      = errors.New("invalid GLB 2.0 structure")
	errBounds      = errors.New("GLB exceeds static v1 resource bounds")
	errUnsupported = errors.New("feature is outside the static GLB v1 profile")
)

func readGLB(data []byte) (gltf, []byte, error) {
	if len(data) < glbHeader || len(data) > maxSourceBytes || string(data[:4]) != "glTF" ||
		binary.LittleEndian.Uint32(data[4:]) != 2 || uint64(binary.LittleEndian.Uint32(data[8:])) != uint64(len(data)) {
		return gltf{}, nil, errFormat
	}

	jsonData, offset, err := chunk(data, glbHeader, jsonChunk)
	if err != nil || len(jsonData) > maxJSONBytes {
		return gltf{}, nil, errFormat
	}

	binaryData, end, err := chunk(data, offset, binChunk)
	if err != nil || end != len(data) {
		return gltf{}, nil, errFormat
	}

	decoder := json.NewDecoder(bytes.NewReader(jsonData))
	decoder.UseNumber()

	if err := inspectJSON(decoder, 0); err != nil {
		return gltf{}, nil, err
	}

	if _, err := decoder.Token(); !errors.Is(err, io.EOF) {
		return gltf{}, nil, errFormat
	}

	var document gltf
	if err := json.Unmarshal(jsonData, &document); err != nil {
		return gltf{}, nil, fmt.Errorf("GLB JSON: %w", err)
	}

	if document.Asset.Version != "2.0" || document.Asset.MinVersion != "" && document.Asset.MinVersion != "2.0" {
		return gltf{}, nil, errFormat
	}

	return document, binaryData, nil
}

//nolint:gosec // Chunk offsets/sizes are checked against a <=64 MiB byte slice before integer conversion.
func chunk(data []byte, offset int, expected uint32) ([]byte, int, error) {
	if offset < 0 || offset > len(data)-chunkHeader {
		return nil, 0, errFormat
	}

	size := uint64(binary.LittleEndian.Uint32(data[offset:]))
	if size%wordSize != 0 || binary.LittleEndian.Uint32(data[offset+wordSize:]) != expected || size > uint64(len(data)-offset-chunkHeader) {
		return nil, 0, errFormat
	}

	start := offset + chunkHeader
	end := start + int(size)

	return data[start:end], end, nil
}

func inspectJSON(decoder *json.Decoder, depth int) error {
	if depth > maxJSONDepth {
		return errBounds
	}

	token, err := decoder.Token()
	if err != nil {
		return fmt.Errorf("GLB JSON: %w", err)
	}

	delimiter, container := token.(json.Delim)
	if !container {
		return nil
	}

	if delimiter != '{' && delimiter != '[' {
		return errFormat
	}

	keys := make(map[string]struct{})

	count := 0
	for decoder.More() {
		count++
		if count > maxObjects {
			return errBounds
		}

		if delimiter == '{' {
			keyToken, keyErr := decoder.Token()

			key, stringKey := keyToken.(string)
			if keyErr != nil || !stringKey {
				return errFormat
			}

			if _, duplicate := keys[strings.ToLower(key)]; duplicate {
				return fmt.Errorf("duplicate JSON key %q: %w", key, errFormat)
			}

			if key == "extensions" {
				return fmt.Errorf("GLB extensions: %w", errUnsupported)
			}

			keys[strings.ToLower(key)] = struct{}{}
		}

		if err := inspectJSON(decoder, depth+1); err != nil {
			return err
		}
	}

	_, err = decoder.Token()

	return err
}
