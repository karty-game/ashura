//go:build !js

package meshopt

import (
	"context"

	"github.com/karty-game/ashura/internal/meshdata"
	"github.com/karty-game/ashura/internal/modeldata"
)

func admitModel(ctx context.Context, source modeldata.Document) error {
	if ctx == nil || len(source.Parts) == 0 || len(source.Parts) > modeldata.MaxParts || len(source.Textures) > modeldata.MaxTextures {
		return ErrInvalid
	}

	var vertices, indices int

	for _, part := range source.Parts {
		if err := ctx.Err(); err != nil {
			return err
		}

		if _, err := meshdata.FromOwned(part.Vertices, part.Indices); err != nil {
			return err
		}

		vertices += len(part.Vertices) / meshdata.VertexStride

		indices += len(part.Indices) / meshdata.IndexStride
		if vertices > meshdata.MaxVertices || indices > meshdata.MaxIndices {
			return ErrInvalid
		}
	}

	return nil
}
