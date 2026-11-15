//nolint:mnd,varnamelen // Scene schema sections and binary field widths retain explicit wire-layout notation.
package lightmapper

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
)

const sceneMagic = "KLMPSCN3"
const maxSceneArtifact = 256 << 20

type sceneMaterialWire struct {
	Reflectance, Emission Vec3
	Texture               uint32
	DoubleSided           bool
}

func materialWireRecords(materials []Material) []sceneMaterialWire {
	result := make([]sceneMaterialWire, len(materials))
	for i, m := range materials {
		result[i] = sceneMaterialWire{m.Reflectance, m.Emission, m.Texture, m.DoubleSided}
	}

	return result
}

//nolint:gocognit,cyclop,gosec,funlen //  Section/count bounds precede every conversion in this versioned binary writer. The versioned source sections are emitted together in their audited wire order.
func encodeSceneSource(s Scene) ([]byte, error) {
	var b bytes.Buffer

	write := func(v any) error { return binary.Write(&b, binary.LittleEndian, v) }
	for _, v := range []string{s.ID, s.Space} {
		if len(v) > 4096 {
			return nil, ErrBounds
		}

		if err := write(uint32(len(v))); err != nil {
			return nil, err
		}

		b.WriteString(v)
	}

	if err := write(s.MetresPerUnit); err != nil {
		return nil, err
	}

	for _, v := range []any{s.Triangles, materialWireRecords(s.Materials), s.Emitters} {
		var n int

		switch a := v.(type) {
		case []Triangle:
			n = len(a)
		case []sceneMaterialWire:
			n = len(a)
		case []Emitter:
			n = len(a)
		}

		if err := write(uint32(n)); err != nil {
			return nil, err
		}

		if err := write(v); err != nil {
			return nil, err
		}

		if _, ok := v.([]sceneMaterialWire); ok {
			for _, m := range s.Materials {
				if len(m.ID) > 4096 {
					return nil, ErrBounds
				}

				if err := write(uint32(len(m.ID))); err != nil {
					return nil, err
				}

				b.WriteString(m.ID)

				if b.Len() > maxSceneArtifact {
					return nil, ErrBounds
				}
			}
		}
	}

	if err := write(uint32(len(s.Textures))); err != nil {
		return nil, err
	}

	for _, t := range s.Textures {
		for _, v := range []any{t.Width, t.Height, t.Pixels} {
			if err := write(v); err != nil {
				return nil, err
			}
		}

		if b.Len() > maxSceneArtifact {
			return nil, ErrBounds
		}
	}

	if err := write(s.Environment); err != nil {
		return nil, err
	}

	if b.Len() > maxSceneArtifact {
		return nil, ErrBounds
	}

	return b.Bytes(), nil
}

//nolint:funlen,gocognit,gocyclo,gosec,cyclop //  Fixed numeric record sizes and nonnegative reader lengths bound every allocation. Admit every bounded wire section before exposing a prepared source scene.
func decodeSceneSource(payload []byte) (Scene, error) {
	r := bytes.NewReader(payload)

	var s Scene

	read := func(v any) error { return binary.Read(r, binary.LittleEndian, v) }
	for _, v := range []*string{&s.ID, &s.Space} {
		var n uint32
		if read(&n) != nil || n > 4096 || uint64(n) > uint64(r.Len()) {
			return Scene{}, ErrBounds
		}

		b := make([]byte, n)
		if _, err := io.ReadFull(r, b); err != nil {
			return Scene{}, err
		}

		*v = string(b)
	}

	if err := read(&s.MetresPerUnit); err != nil {
		return Scene{}, err
	}

	for kind := range 3 {
		var n uint32
		if read(&n) != nil || n > 1<<20 {
			return Scene{}, ErrBounds
		}

		var (
			target any
			size   int
		)

		switch kind {
		case 0:
			size = binary.Size(Triangle{})
			if uint64(n)*uint64(size) > uint64(r.Len()) {
				return Scene{}, ErrBounds
			}

			s.Triangles = make([]Triangle, n)
			target = s.Triangles
		case 1:
			size = binary.Size(sceneMaterialWire{})
			if uint64(n)*uint64(size) > uint64(r.Len()) {
				return Scene{}, ErrBounds
			}

			fixed := make([]sceneMaterialWire, n)
			if read(fixed) != nil {
				return Scene{}, ErrInvalid
			}

			s.Materials = make([]Material, n)

			for i, m := range fixed {
				var length uint32
				if read(&length) != nil || length > 4096 || uint64(length) > uint64(r.Len()) {
					return Scene{}, ErrBounds
				}

				name := make([]byte, length)
				if _, err := io.ReadFull(r, name); err != nil {
					return Scene{}, err
				}

				s.Materials[i] = Material{
					ID:          string(name),
					Reflectance: m.Reflectance,
					Emission:    m.Emission,
					Texture:     m.Texture,
					DoubleSided: m.DoubleSided,
				}
			}

			continue
		case 2:
			size = binary.Size(Emitter{})
			if uint64(n)*uint64(size) > uint64(r.Len()) {
				return Scene{}, ErrBounds
			}

			s.Emitters = make([]Emitter, n)
			target = s.Emitters
		}

		if err := read(target); err != nil {
			return Scene{}, fmt.Errorf("scene array: %w", ErrInvalid)
		}
	}

	var n uint32
	if read(&n) != nil || n > 1<<20 || uint64(n)*8 > uint64(r.Len()) {
		return Scene{}, ErrBounds
	}

	s.Textures = make([]Texture, n)
	for i := range s.Textures {
		t := &s.Textures[i]
		if read(&t.Width) != nil || read(&t.Height) != nil {
			return Scene{}, ErrInvalid
		}

		pixels := uint64(t.Width) * uint64(t.Height)
		if pixels > 1<<24 || pixels*12 > uint64(r.Len()) {
			return Scene{}, ErrBounds
		}

		t.Pixels = make([]Vec3, int(pixels))
		if read(t.Pixels) != nil {
			return Scene{}, ErrInvalid
		}
	}

	if read(&s.Environment) != nil || r.Len() != 0 {
		return Scene{}, ErrInvalid
	}

	return s, nil
}
