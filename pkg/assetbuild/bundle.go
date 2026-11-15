//go:build !js

package assetbuild

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"slices"
)

const (
	bundleMagic      = "KARTBND\x00"
	bundleVersion    = 1
	bundleHeaderSize = 48
	bundleEntrySize  = 8
	bundleMaxFiles   = 65536
	bundleMaxName    = 4096
)

// EncodeBundle snapshots generated files in the deterministic binary bundle v1
// format. Payloads remain raw bytes, avoiding JSON's base64 expansion. Complete
// bundles and individual files retain the 256 MiB artifact admission bound; names
// are bounded at 4096 bytes and a binary bundle holds at most 65536 files.
// Legacy JSON bundles remain accepted by Plan.Publish for existing processors.
//
//nolint:gosec // Complete preflight bounds names, file counts and lengths below 256 MiB before encoding uint32 words.
func EncodeBundle(files Bundle) ([]byte, error) {
	if err := validateOutputRoster(files); err != nil {
		return nil, err
	}

	if len(files) > bundleMaxFiles {
		return nil, fmt.Errorf("generated bundle has too many files: %w", errPlan)
	}

	size := uint64(bundleHeaderSize)

	names := make([]string, 0, len(files))
	for name, data := range files {
		if len(name) > bundleMaxName || len(data) > maxArtifactBytes {
			return nil, fmt.Errorf("generated bundle entry exceeds bounds: %w", errPlan)
		}

		size += bundleEntrySize + uint64(len(name)) + uint64(len(data))
		if size > maxArtifactBytes {
			return nil, fmt.Errorf("generated binary bundle needs %d bytes, exceeds %d-byte limit: %w", size, maxArtifactBytes, errPlan)
		}

		names = append(names, name)
	}

	slices.Sort(names)

	encoded := make([]byte, int(size))
	copy(encoded, bundleMagic)
	binary.LittleEndian.PutUint32(encoded[8:], bundleVersion)
	binary.LittleEndian.PutUint32(encoded[12:], uint32(len(names)))

	offset := bundleHeaderSize

	for _, name := range names {
		data := files[name]
		binary.LittleEndian.PutUint32(encoded[offset:], uint32(len(name)))
		binary.LittleEndian.PutUint32(encoded[offset+4:], uint32(len(data)))
		offset += 8
		offset += copy(encoded[offset:], name)
		offset += copy(encoded[offset:], data)
	}

	sum := bundleChecksum(encoded)
	copy(encoded[16:48], sum[:])

	return encoded, nil
}

func decodeBundle(encoded []byte) (Bundle, error) {
	if len(encoded) > maxArtifactBytes {
		return nil, fmt.Errorf("generated bundle exceeds byte limit: %w", errPlan)
	}

	if len(encoded) >= len(bundleMagic) && string(encoded[:len(bundleMagic)]) == bundleMagic {
		return decodeBinaryBundle(encoded)
	}

	var files Bundle
	if err := json.Unmarshal(encoded, &files); err != nil {
		return nil, fmt.Errorf("decode legacy generated bundle: %w", err)
	}

	if err := validateOutputRoster(files); err != nil {
		return nil, err
	}

	return files, nil
}

func decodeBinaryBundle(encoded []byte) (Bundle, error) {
	if len(encoded) < bundleHeaderSize || binary.LittleEndian.Uint32(encoded[8:]) != bundleVersion {
		return nil, errPlan
	}

	count := binary.LittleEndian.Uint32(encoded[12:])
	if count > bundleMaxFiles || int64(count)*(bundleEntrySize+1) > int64(len(encoded)-bundleHeaderSize) {
		return nil, errPlan
	}

	var expected [32]byte
	copy(expected[:], encoded[16:48])

	if bundleChecksum(encoded) != expected {
		return nil, errPlan
	}

	files := make(Bundle, count)
	offset := bundleHeaderSize
	previous := ""

	for range count {
		if offset > len(encoded)-8 {
			return nil, errPlan
		}

		nameSize, dataSize := binary.LittleEndian.Uint32(encoded[offset:]), binary.LittleEndian.Uint32(encoded[offset+4:])

		offset += 8
		if nameSize == 0 || nameSize > bundleMaxName || int64(nameSize)+int64(dataSize) > int64(len(encoded)-offset) {
			return nil, errPlan
		}

		name := string(encoded[offset : offset+int(nameSize)])
		offset += int(nameSize)

		if !generatedName(name) || name <= previous {
			return nil, errPath
		}

		previous = name
		files[name] = encoded[offset : offset+int(dataSize) : offset+int(dataSize)]
		offset += int(dataSize)
	}

	if offset != len(encoded) {
		return nil, errPlan
	}

	if err := validateOutputRoster(files); err != nil {
		return nil, err
	}

	return files, nil
}

func bundleChecksum(encoded []byte) [32]byte {
	hash := sha256.New()
	_, _ = hash.Write(encoded[:16])
	_, _ = hash.Write(encoded[48:])

	var sum [32]byte
	copy(sum[:], hash.Sum(nil))

	return sum
}
