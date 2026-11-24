package particledata

import (
	"bytes"
	"encoding/binary"
	"math"
	"testing"
)

func TestReadLegacyParticleBundle(t *testing.T) {
	t.Parallel()

	var data bytes.Buffer
	if err := binary.Write(&data, binary.LittleEndian, header{
		Magic: magicV1, Width: 1, Height: 1, Layers: 1, Duration: 1,
	}); err != nil {
		t.Fatal(err)
	}

	if err := binary.Write(&data, binary.LittleEndian, layerHeader{Capacity: 1, MaxLife: 1}); err != nil {
		t.Fatal(err)
	}

	oldConfig := make([]byte, SpatialOffset)
	binary.LittleEndian.PutUint32(oldConfig, math.Float32bits(1))
	binary.LittleEndian.PutUint32(oldConfig[4:], math.Float32bits(1))
	data.Write(oldConfig)
	data.Write([]byte{255, 255, 255, 255})

	effect, err := Read(&data)
	if err != nil {
		t.Fatal(err)
	}

	config := effect.Layers[0].Config
	if len(config) != ConfigBytes || !bytes.Equal(config[:SpatialOffset], oldConfig) ||
		!bytes.Equal(config[SpatialOffset:], make([]byte, ConfigBytes-SpatialOffset)) {
		t.Fatal("version 1 layer did not retain its planar configuration")
	}
}
