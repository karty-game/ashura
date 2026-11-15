package basistranscode

import (
	_ "embed"
	"encoding/binary"
	"strconv"
	"testing"
)

//go:embed testdata/albedo.ktx2
var testAlbedo []byte

func TestInspectETC1SBounds(t *testing.T) {
	t.Parallel()

	data := testAlbedo

	info, err := Inspect(data)
	if err != nil || info != (Info{8, 8, 4}) {
		t.Fatalf("profile: %+v, %v", info, err)
	}

	for _, offset := range []int{12, 16, 20, 24, 28, 32, 36, 40, 44, 48, 52, 56, 60, 64, 72, 80, 88, 96} {
		t.Run(strconv.Itoa(offset), func(t *testing.T) {
			t.Parallel()

			bad := append([]byte(nil), data...)
			binary.LittleEndian.PutUint32(bad[offset:], 0xffffffff)

			if _, err := Inspect(bad); err == nil {
				t.Fatalf("accepted invalid header/range at %d", offset)
			}
		})
	}

	for _, length := range []int{0, 12, 79, 80, 175, len(data) - 1} {
		if _, err := Inspect(data[:length]); err == nil {
			t.Fatalf("accepted truncated %d", length)
		}
	}
}
