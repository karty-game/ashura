//go:build !js

package assetbuild

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"testing"
)

func TestBinaryBundleOwnsPayloadAndCollectsWithLegacyJSON(t *testing.T) {
	t.Parallel()

	files := Bundle{"new.bin": []byte("complete")}

	encoded, err := EncodeBundle(files)
	if err != nil {
		t.Fatal(err)
	}

	clear(files["new.bin"])

	decoded, err := decodeBundle(encoded)
	if err != nil || string(decoded["new.bin"]) != "complete" {
		t.Fatalf("binary bundle lost owned snapshot: %q %v", decoded["new.bin"], err)
	}

	legacy, err := json.Marshal(Bundle{"old.bin": []byte("legacy")})
	if err != nil {
		t.Fatal(err)
	}

	first, second := Ref{Action: "old", Slot: "files"}, Ref{Action: "new", Slot: "files"}
	run := runner{outputs: map[Ref]outputValue{first: {data: legacy}, second: {data: encoded}}}

	collected, err := run.collect([]Ref{first, second})
	if err != nil || string(collected["old.bin"]) != "legacy" || string(collected["new.bin"]) != "complete" {
		t.Fatalf("mixed bundle compatibility: %#v %v", collected, err)
	}
}

func TestBinaryBundleRejectsMalformedAndTruncatedPayloads(t *testing.T) {
	t.Parallel()

	encoded, err := EncodeBundle(Bundle{"a.bin": []byte("first"), "b.bin": []byte("second")})
	if err != nil {
		t.Fatal(err)
	}

	for length := range encoded {
		if _, err := decodeBundle(encoded[:length]); err == nil {
			t.Fatalf("accepted binary truncation%d", length)
		}
	}

	for _, name := range []string{"version", "count", "name-size", "payload-size", "unsafe-name", "unsorted-name", "checksum", "trailing"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			data := bytes.Clone(encoded)

			switch name {
			case "version":
				binary.LittleEndian.PutUint32(data[8:], 2)
			case "count":
				binary.LittleEndian.PutUint32(data[12:], bundleMaxFiles+1)
			case "name-size":
				binary.LittleEndian.PutUint32(data[bundleHeaderSize:], bundleMaxName+1)
			case "payload-size":
				binary.LittleEndian.PutUint32(data[bundleHeaderSize+4:], ^uint32(0))
			case "unsafe-name":
				copy(data[bundleHeaderSize+8:], "../ab")
			case "unsorted-name":
				copy(data[bundleHeaderSize+8:], "z.bin")
			case "checksum":
				data[len(data)-1] ^= 1
			case "trailing":
				data = append(data, 0)
			}

			if name != "checksum" {
				sum := bundleChecksum(data)
				copy(data[16:48], sum[:])
			}

			if _, err := decodeBundle(data); err == nil {
				t.Fatal("accepted malformed binary bundle")
			}
		})
	}
}

func TestBinaryBundleRejectsConflictingRosterAndEncodingBounds(t *testing.T) {
	t.Parallel()

	for _, files := range []Bundle{{"a": nil, "a/b": nil}, {"../escape": nil}, {manifestName: nil}} {
		if _, err := EncodeBundle(files); err == nil {
			t.Fatal("encoded unsafe output roster")
		}
	}

	encoded, err := EncodeBundle(Bundle{"a": nil, "b/c": nil})
	if err != nil {
		t.Fatal(err)
	}
	// Each entry has8bytes of lengths followed by its name and raw payload.
	copy(encoded[bundleHeaderSize+8+1+8:], "a/c")
	sum := bundleChecksum(encoded)
	copy(encoded[16:48], sum[:])

	if _, err := decodeBundle(encoded); err == nil {
		t.Fatal("accepted conflicting decoded roster")
	}
}

//nolint:paralleltest // Serialize the200MiB binary-bundle admission regression to bound test memory.
func TestBinaryBundleAvoidsBase64ExpansionWithinUnchangedArtifactBound(t *testing.T) {
	first, second := make([]byte, maxArtifactBytes*2/5), make([]byte, maxArtifactBytes*2/5)

	first[0], second[0] = 17, 42
	if (len(first)+len(second))*4/3 <= maxArtifactBytes {
		t.Fatal("fixture does not exceed legacy JSON bound")
	}

	encoded, err := EncodeBundle(Bundle{"first.bin": first, "second.bin": second})
	if err != nil {
		t.Fatal(err)
	}

	if len(encoded) > maxArtifactBytes || len(encoded) != len(first)+len(second)+bundleHeaderSize+16+len("first.binsecond.bin") {
		t.Fatal("binary bundle changed byte admission bound or expanded payloads")
	}

	decoded, err := decodeBundle(encoded)
	if err != nil || !bytes.Equal(decoded["first.bin"], first) || !bytes.Equal(decoded["second.bin"], second) {
		t.Fatalf("large binary bundle changed payloads: %v", err)
	}
}
