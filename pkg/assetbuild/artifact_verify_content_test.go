//go:build !js

package assetbuild

import (
	"bytes"
	"errors"
	"io"
	"testing"
)

var errArtifactStreamIO = errors.New("artifact stream I/O fixture")

type countedArtifactReader struct {
	data                 []byte
	used, maximum, calls int
}

func (reader *countedArtifactReader) Read(target []byte) (int, error) {
	reader.calls++

	reader.maximum = max(reader.maximum, len(target))
	if len(reader.data) == 0 {
		return 0, io.EOF
	}

	count := copy(target, reader.data)
	reader.data = reader.data[count:]
	reader.used += count

	return count, nil
}

func TestArtifactContentStreamingParityAndBoundedScratch(t *testing.T) {
	t.Parallel()

	var priorScratch int

	for _, size := range []int{0, 1, 1 << 20, 2 << 20} {
		data := bytes.Repeat([]byte{37}, size)
		reader := &countedArtifactReader{data: data}

		matched, err := artifactContentMatches(reader, Artifact{SHA256: digest(data), Size: len(data)})
		if err != nil || !matched || reader.used != size {
			t.Fatal("valid streaming checksum changed", size, matched, err)
		}

		if size >= 1<<20 {
			if reader.maximum >= size {
				t.Fatal("admission allocated an artifact-sized read buffer", size, reader.maximum)
			}

			if priorScratch != 0 && priorScratch != reader.maximum {
				t.Fatal("admission scratch grew with the artifact", priorScratch, reader.maximum)
			}

			priorScratch = reader.maximum
		}
	}
}

func TestArtifactContentRejectsTruncationGrowthAndWrongDigest(t *testing.T) {
	t.Parallel()

	data := []byte("immutable artifact")

	artifact := Artifact{SHA256: digest(data), Size: len(data)}
	for _, actual := range [][]byte{data[:len(data)-1], append(append([]byte(nil), data...), 1), bytes.Repeat([]byte{0}, len(data))} {
		if matched, err := artifactContentMatches(bytes.NewReader(actual), artifact); err != nil || matched {
			t.Fatal("changed stream accepted", matched, err)
		}
	}

	reader := &countedArtifactReader{data: bytes.Repeat([]byte{37}, 2<<20)}
	if matched, err := artifactContentMatches(
		reader,
		Artifact{SHA256: digest(reader.data[:1<<20]), Size: 1 << 20},
	); err != nil || matched ||
		reader.used != (1<<20)+1 {
		t.Fatal("growth was accepted or read past its one-byte sentinel", matched, reader.used, err)
	}
}

type artifactErrorReader struct {
	data    []byte
	failure error
}

func (reader artifactErrorReader) Read(target []byte) (int, error) {
	return copy(target, reader.data), reader.failure
}

func TestArtifactContentPropagatesIOErrorsAndRejectsInvalidReferencesBeforeRead(t *testing.T) {
	t.Parallel()

	failure := errArtifactStreamIO
	data := []byte("payload")

	artifact := Artifact{SHA256: digest(data), Size: len(data)}
	if matched, err := artifactContentMatches(artifactErrorReader{data: data, failure: io.EOF}, artifact); err != nil || !matched {
		t.Fatal("a final complete read carrying EOF changed checksum admission", matched, err)
	}

	for _, reader := range []io.Reader{
		artifactErrorReader{failure: failure},
		artifactErrorReader{data: data, failure: failure},
		io.MultiReader(bytes.NewReader(data), artifactErrorReader{failure: failure}),
	} {
		if matched, err := artifactContentMatches(reader, artifact); matched || !errors.Is(err, failure) {
			t.Fatal("I/O failure was discarded", matched, err)
		}
	}

	for _, invalid := range []Artifact{{SHA256: "../escape", Size: 0}, {SHA256: digest(nil), Size: -1}, {SHA256: digest(nil), Size: maxArtifactBytes + 1}} {
		reader := &countedArtifactReader{data: data}
		if matched, err := artifactContentMatches(reader, invalid); matched || !errors.Is(err, errPlan) || reader.calls != 0 {
			t.Fatal("invalid reference reached stream I/O", matched, err, reader.calls)
		}
	}
}
