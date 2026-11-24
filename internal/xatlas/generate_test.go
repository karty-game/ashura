//go:build !js

package xatlas

import (
	"context"
	"encoding/binary"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/karty-game/ashura/internal/modeldata"
)

func TestPortableSingleAtlasPreservesSourceTriangles(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()

	session, err := OpenSession(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = session.Close() }()

	parts := []modeldata.Part{pageTestQuad("first", 0, 1), pageTestQuad("second", 3, 1)}

	atlas, err := session.Generate(ctx, parts, 64, 4)
	if err != nil {
		t.Fatal(err)
	}

	if len(atlas.Meshes) != len(parts) || atlas.Width == 0 || atlas.Height == 0 || atlas.Charts == 0 {
		t.Fatal("portable single-atlas library omitted source meshes or chart metadata")
	}

	for index, mesh := range atlas.Meshes {
		source := parts[index]
		if len(mesh.Indices) != len(source.Indices)/4 {
			t.Fatal("portable library dropped source triangles")
		}

		for corner, vertex := range mesh.Indices {
			original := binary.LittleEndian.Uint32(source.Indices[corner*4:])
			if mesh.Vertices[vertex].Original != original {
				t.Fatal("portable library changed source-face index mapping")
			}
		}
	}

	again, err := session.Generate(ctx, parts, 64, 4)
	if err != nil || !reflect.DeepEqual(atlas, again) {
		t.Fatal("portable single-atlas packing is not deterministic", err)
	}
}

func TestPortableSingleAtlasRequiresLiveSession(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	parts := []modeldata.Part{pageTestQuad("single atlas", 0, 1)}

	session, err := OpenSession(t.Context())
	if err != nil {
		t.Fatal(err)
	}

	if _, err := session.Generate(ctx, parts, 64, 4); !errors.Is(err, context.Canceled) {
		t.Fatal("canceled single-atlas generation invoked the helper", err)
	}

	if err := session.Close(); err != nil {
		t.Fatal(err)
	}

	if err := session.Close(); err != nil {
		t.Fatal("repeated close failed", err)
	}

	if _, err := session.Generate(t.Context(), parts, 64, 4); err == nil {
		t.Fatal("closed single-atlas session remained usable")
	}
}
