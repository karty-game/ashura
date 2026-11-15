//go:build !js

package assetbuild

import (
	"bytes"
	"errors"
	"io"
	"testing"
)

func TestSizedBoundedReadPreservesContentsWhenSizeChanges(t *testing.T) {
	t.Parallel()

	content := bytes.Repeat([]byte("payload"), 1024)
	for _, expected := range []int64{0, 1, 512, int64(len(content)), int64(len(content) + 512)} {
		actual, err := readSizedBounded(bytes.NewReader(content), expected)
		if err != nil || !bytes.Equal(actual, content) {
			t.Fatal("sized read changed content", expected, err)
		}
	}

	empty, err := readSizedBounded(bytes.NewReader(nil), 0)
	if err != nil || len(empty) != 0 {
		t.Fatal("empty sized read failed", err)
	}

	for _, expected := range []int64{-1, maxArtifactBytes + 1} {
		if _, err = readSizedBounded(bytes.NewReader(nil), expected); !errors.Is(err, errPath) {
			t.Fatal("unbounded allocation admitted", expected, err)
		}
	}
}

type failingBoundedReader struct{ err error }

func (reader failingBoundedReader) Read([]byte) (int, error) { return 0, reader.err }
func TestSizedBoundedReadPropagatesIOFailure(t *testing.T) {
	t.Parallel()

	for _, failure := range []error{io.ErrClosedPipe, io.ErrUnexpectedEOF} {
		if _, err := readSizedBounded(failingBoundedReader{failure}, 32); !errors.Is(err, failure) {
			t.Fatal("I/O failure discarded", err)
		}
	}

	if _, err := readSizedBounded(&fullErrorReader{}, 0); !errors.Is(err, io.ErrClosedPipe) {
		t.Fatal("full-buffer I/O failure discarded", err)
	}
}

type fullErrorReader struct{ done bool }

func (reader *fullErrorReader) Read(target []byte) (int, error) {
	if reader.done {
		return 0, io.EOF
	}

	reader.done = true

	clear(target)

	return len(target), io.ErrClosedPipe
}
