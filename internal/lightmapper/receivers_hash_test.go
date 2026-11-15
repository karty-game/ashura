package lightmapper

import (
	"math"
	"reflect"
	"slices"
	"testing"
)

func receiverHashClone(t *testing.T, source []Receiver) []Receiver {
	t.Helper()

	result := slices.Clone(source)
	for index := range result {
		receiver := &result[index]

		receiver.Pieces = slices.Clone(receiver.Pieces)
		for pieceIndex := range receiver.Pieces {
			piece := &receiver.Pieces[pieceIndex]
			piece.Barycentric = slices.Clone(piece.Barycentric)
		}

		receiver.Patches = slices.Clone(receiver.Patches)
	}

	return result
}

func receiverHashLeaves(value reflect.Value, path []int, output *[][]int) {
	//nolint:exhaustive // Only receiver-contract kinds are supported; any new kind must fail this coverage test.
	switch value.Kind() {
	case reflect.Struct:
		for i := range value.NumField() {
			receiverHashLeaves(value.Field(i), append(path, i), output)
		}
	case reflect.Array, reflect.Slice:
		for i := range value.Len() {
			receiverHashLeaves(value.Index(i), append(path, i), output)
		}
	case reflect.Uint32, reflect.Uint64, reflect.Int32, reflect.Float32:
		*output = append(*output, append([]int(nil), path...))
	default:
		panic("receiver identity gained an uncovered field kind")
	}
}
func receiverHashLeaf(root reflect.Value, path []int) reflect.Value {
	for _, index := range path {
		if root.Kind() == reflect.Struct {
			root = root.Field(index)
		} else {
			root = root.Index(index)
		}
	}

	return root
}

// Reflection enumerates the actual contract rather than duplicating the hash's
// field list: every current or newly added scalar must affect the identity.
func TestReceiverHashBindsEveryNestedField(t *testing.T) {
	t.Parallel()

	receiver := testPointReceiver()
	receiver.Pieces = []FootprintPiece{{Triangle: 3, Barycentric: []Vec3{{1, 0, 0}, {0, 1, 0}, {0, 0, 1}}, Area: .5}}
	source := []Receiver{receiver}

	expected, err := ReceiverHash(source)
	if err != nil {
		t.Fatal(err)
	}

	var paths [][]int
	receiverHashLeaves(reflect.ValueOf(source), nil, &paths)

	for _, path := range paths {
		changed := receiverHashClone(t, source)
		leaf := receiverHashLeaf(reflect.ValueOf(changed), path)
		//nolint:exhaustive // receiverHashLeaves admits only these scalar kinds.
		switch leaf.Kind() {
		case reflect.Uint32, reflect.Uint64:
			leaf.SetUint(leaf.Uint() + 1)
		case reflect.Int32:
			leaf.SetInt(leaf.Int() + 1)
		case reflect.Float32:
			leaf.SetFloat(leaf.Float() + 1)
		}

		actual, hashErr := ReceiverHash(changed)
		if hashErr != nil || actual == expected {
			t.Fatal("receiver field was not bound", path, hashErr)
		}
	}

	actual, err := ReceiverHash(source)
	if err != nil || actual != expected {
		t.Fatal("hashing mutated the input", err)
	}
}

func TestReceiverHashPreservesSlicePresenceAndFloatBits(t *testing.T) {
	t.Parallel()

	hash := func(receivers []Receiver) string {
		t.Helper()

		value, err := ReceiverHash(receivers)
		if err != nil {
			t.Fatal(err)
		}

		return value
	}
	if hash(nil) == hash([]Receiver{}) {
		t.Fatal("nil receiver set aliases empty set")
	}

	receiver := testPointReceiver()
	receiver.Pieces = nil
	nilPieces := hash([]Receiver{receiver})

	receiver.Pieces = []FootprintPiece{}
	if hash([]Receiver{receiver}) == nilPieces {
		t.Fatal("nil footprint list aliases empty list")
	}

	receiver.Position[0] = 0
	positive := hash([]Receiver{receiver})

	receiver.Position[0] = math.Float32frombits(1 << 31)
	if hash([]Receiver{receiver}) == positive {
		t.Fatal("signed zero frame bits lost")
	}
}

func TestReceiverHashRejectsNonfiniteNestedValues(t *testing.T) {
	t.Parallel()

	source := []Receiver{testPointReceiver()}
	source[0].Pieces = []FootprintPiece{{Triangle: 3, Barycentric: []Vec3{{1, 0, 0}, {0, 1, 0}, {0, 0, 1}}, Area: .5}}

	var paths [][]int
	receiverHashLeaves(reflect.ValueOf(source), nil, &paths)

	for _, path := range paths {
		if receiverHashLeaf(reflect.ValueOf(source), path).Kind() != reflect.Float32 {
			continue
		}

		for _, bad := range []float64{math.NaN(), math.Inf(1), math.Inf(-1)} {
			changed := receiverHashClone(t, source)
			receiverHashLeaf(reflect.ValueOf(changed), path).SetFloat(bad)

			if _, err := ReceiverHash(changed); err == nil {
				t.Fatal("nonfinite nested frame accepted", path)
			}
		}
	}
}
