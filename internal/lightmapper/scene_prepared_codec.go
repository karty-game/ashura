package lightmapper

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
)

func writePreparedSlice(buffer *bytes.Buffer, values any, count int) error {
	if count < 0 || count > maxSceneArtifact {
		return ErrBounds
	}

	if err := binary.Write(buffer, binary.LittleEndian, uint32(count)); err != nil {
		return err
	}

	if err := binary.Write(buffer, binary.LittleEndian, values); err != nil {
		return err
	}

	if buffer.Len() > maxSceneArtifact {
		return ErrBounds
	}

	return nil
}

//nolint:gosec // Fixed record widths and nonnegative reader lengths precede bounded allocation.
func readPreparedSlice[T any](reader *bytes.Reader, maximum uint64) ([]T, error) {
	var count uint32
	if binary.Read(reader, binary.LittleEndian, &count) != nil {
		return nil, ErrInvalid
	}

	var value T

	size := binary.Size(value)
	if size <= 0 || uint64(count) > maximum || uint64(count)*uint64(size) > uint64(reader.Len()) {
		return nil, ErrBounds
	}

	values := make([]T, int(count))
	if binary.Read(reader, binary.LittleEndian, values) != nil {
		return nil, ErrInvalid
	}

	return values, nil
}

// EncodeScene persists actual prepared acceleration and exact sampling data.
// Source and arrays occupy one checksummed binary artifact; no caller must
// reconstruct a BVH merely to reuse a cached scene on a later build.
//
//nolint:gosec // Source and complete payload lengths are capped at256MiB before uint32 framing.
func EncodeScene(prepared *PreparedScene) ([]byte, error) {
	if prepared == nil {
		return nil, ErrInvalid
	}

	if err := prepared.validatePreparedAcceleration(); err != nil {
		return nil, err
	}

	source, err := encodeSceneSource(prepared.source)
	if err != nil {
		return nil, err
	}

	var payload bytes.Buffer
	if binary.Write(&payload, binary.LittleEndian, uint32(len(source))) != nil {
		return nil, ErrInvalid
	}

	payload.Write(source)

	for _, record := range []struct {
		data  any
		count int
	}{{prepared.nodes, len(prepared.nodes)}, {prepared.order, len(prepared.order)}, {prepared.lightNodes, len(prepared.lightNodes)}, {prepared.lightOrder, len(prepared.lightOrder)}, {prepared.lightLeaves, len(prepared.lightLeaves)}, {prepared.lightParents, len(prepared.lightParents)}, {prepared.environmentRows, len(prepared.environmentRows)}, {prepared.environmentColumns, len(prepared.environmentColumns)}} {
		if err = writePreparedSlice(&payload, record.data, record.count); err != nil {
			return nil, err
		}
	}

	sum := sha256.Sum256(payload.Bytes())
	out := make([]byte, 0, 44+payload.Len())
	out = append(out, sceneMagic...)
	out = binary.LittleEndian.AppendUint32(out, uint32(payload.Len()))
	out = append(out, sum[:]...)
	out = append(out, payload.Bytes()...)

	return out, nil
}

// DecodeScene admits stored acceleration without rebuilding it. Structural,
// ownership, containment and sampling-support checks precede GPU publication.
//
//nolint:cyclop,funlen,gosec // Every independently bounded arena is admitted in binary protocol order.
func DecodeScene(encoded []byte) (*PreparedScene, error) {
	if len(encoded) < 48 || len(encoded) > maxSceneArtifact+44 || string(encoded[:8]) != sceneMagic ||
		uint64(binary.LittleEndian.Uint32(encoded[8:])) != uint64(len(encoded)-44) {
		return nil, ErrInvalid
	}

	sum := sha256.Sum256(encoded[44:])
	if !bytes.Equal(sum[:], encoded[12:44]) {
		return nil, ErrInvalid
	}

	reader := bytes.NewReader(encoded[44:])

	var sourceSize uint32
	if binary.Read(reader, binary.LittleEndian, &sourceSize) != nil || uint64(sourceSize) > uint64(reader.Len()) {
		return nil, ErrBounds
	}

	sourceBytes := make([]byte, sourceSize)
	if _, err := reader.Read(sourceBytes); err != nil {
		return nil, ErrInvalid
	}

	source, err := decodeSceneSource(sourceBytes)
	if err != nil {
		return nil, err
	}

	prepared, err := admitSceneSource(source, true)
	if err != nil {
		return nil, err
	}

	geometryCount, lightCount := uint64(len(source.Triangles)), uint64(len(prepared.source.Emitters))
	if prepared.nodes, err = readPreparedSlice[BVHNode](reader, geometryCount*2); err != nil {
		return nil, err
	}

	if prepared.order, err = readPreparedSlice[uint32](reader, geometryCount); err != nil {
		return nil, err
	}

	if prepared.lightNodes, err = readPreparedSlice[LightNode](reader, lightCount*2); err != nil {
		return nil, err
	}

	if prepared.lightOrder, err = readPreparedSlice[uint32](reader, lightCount); err != nil {
		return nil, err
	}

	if prepared.lightLeaves, err = readPreparedSlice[uint32](reader, lightCount); err != nil {
		return nil, err
	}

	if prepared.lightParents, err = readPreparedSlice[uint32](reader, lightCount*2); err != nil {
		return nil, err
	}

	var rows, columns uint64

	if prepared.source.Environment.Texture > 0 {
		t := prepared.source.Textures[prepared.source.Environment.Texture-1]
		rows = uint64(t.Height) + 1
		columns = uint64(t.Height) * (uint64(t.Width) + 1)
	}

	if prepared.environmentRows, err = readPreparedSlice[float32](reader, rows); err != nil {
		return nil, err
	}

	if prepared.environmentColumns, err = readPreparedSlice[float32](reader, columns); err != nil {
		return nil, err
	}

	if reader.Len() != 0 {
		return nil, ErrInvalid
	}

	if err = prepared.validatePreparedAcceleration(); err != nil {
		return nil, err
	}

	if err = prepared.validateSurfaceOwnership(); err != nil {
		return nil, err
	}

	if err = prepared.finishSceneIdentity(); err != nil {
		return nil, err
	}

	return prepared, nil
}
