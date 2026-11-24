//go:build !js

package collisionbuild

import (
	"encoding/binary"
	"fmt"
	"math"
	"path"
	"strings"

	"github.com/karty-game/ashura/internal/meshdata"
	"github.com/karty-game/ashura/internal/modeldata"
	"github.com/karty-game/ashura/pkg/assetbuild"
	"github.com/karty-game/ashura/pkg/collision"
	"github.com/karty-game/ashura/pkg/engine"
)

type modelPublisher struct{}

// StaticModels publishes prepared model resources as static collision meshes.
// Supply explicit collision proxy GLBs with modelbuild.PreparedGLB in the same
// pipeline. The importer flattens the source node transforms during the build.
func StaticModels() assetbuild.Plugin { return modelPublisher{} }

func (modelPublisher) Name() string { return "model-collision" }

func (publisher modelPublisher) Plan(plan *assetbuild.Plan) error {
	return plan.Expand(assetbuild.Expansion{
		ID: "model-collision/discover",
		Run: func(_ *assetbuild.Context, child *assetbuild.Plan) error {
			return publisher.models(child)
		},
	})
}

func (modelPublisher) models(plan *assetbuild.Plan) error {
	names := make(map[string]bool)

	for _, resource := range plan.Resources {
		schema, err := plan.SlotSchema(resource.Ref)
		if err != nil {
			return err
		}

		if schema != modeldata.PreparedSchema {
			continue
		}

		name := assetbuild.AssetName(strings.TrimSuffix(resource.ID, path.Ext(resource.ID)))
		if names[name] {
			return fmt.Errorf("duplicate collision catalog name %s: %w", name, errGeometry)
		}

		names[name] = true

		action := meshAction(name, "model-collision/"+resource.ID, name, []assetbuild.Ref{resource.Ref},
			func(ctx *assetbuild.Context) ([]collision.Triangle, error) {
				data, err := ctx.Input(resource.Ref)
				if err != nil {
					return nil, err
				}

				return modelTriangles(data)
			})
		if err := publish(plan, name, action); err != nil {
			return err
		}
	}

	if len(names) == 0 {
		return fmt.Errorf("no prepared model resources: %w", errGeometry)
	}

	return nil
}

func modelTriangles(data []byte) ([]collision.Triangle, error) {
	model, err := modeldata.Decode(data)
	if err != nil {
		return nil, err
	}

	const triangleStride = 3 * meshdata.IndexStride

	var count int

	for _, part := range model.Parts {
		count += len(part.Indices) / triangleStride
		if count > maxTriangles {
			return nil, errGeometry
		}
	}

	triangles := make([]collision.Triangle, 0, count)

	for _, part := range model.Parts {
		for offset := 0; offset < len(part.Indices); offset += triangleStride {
			indices := part.Indices[offset : offset+triangleStride]
			triangles = append(triangles, collision.Triangle{
				A: position(part.Vertices, binary.LittleEndian.Uint32(indices)),
				B: position(part.Vertices, binary.LittleEndian.Uint32(indices[meshdata.IndexStride:])),
				C: position(part.Vertices, binary.LittleEndian.Uint32(indices[2*meshdata.IndexStride:])),
			})
		}
	}

	return triangles, nil
}

func position(vertices []byte, index uint32) engine.Vector3 {
	const word = meshdata.IndexStride

	start := int(index) * meshdata.VertexStride
	data := vertices[start:]

	return engine.Vector3{
		X: math.Float32frombits(binary.LittleEndian.Uint32(data)),
		Y: math.Float32frombits(binary.LittleEndian.Uint32(data[word:])),
		Z: math.Float32frombits(binary.LittleEndian.Uint32(data[2*word:])),
	}
}
