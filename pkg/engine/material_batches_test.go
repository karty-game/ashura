package engine

import (
	"testing"
	"unsafe"
)

func TestMaterialBatchMapKeyFitsInlineStorage(t *testing.T) {
	t.Parallel()

	const maximumInlineKey = 128
	if bytes := unsafe.Sizeof(materialKey{}); bytes > maximumInlineKey {
		t.Fatalf("material key=%d bytes, map would box keys over128 bytes", bytes)
	}
}
