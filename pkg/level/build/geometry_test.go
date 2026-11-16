//go:build !js

package levelbuild

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"reflect"
	"strconv"
	"testing"

	"github.com/karty-game/karty-webgpu/internal/lightmapdata"
	"github.com/karty-game/karty-webgpu/internal/modeldata"
	"github.com/karty-game/karty-webgpu/pkg/assetbuild"
	bake "github.com/karty-game/karty-webgpu/pkg/lightmap/bake"
	meshbuild "github.com/karty-game/karty-webgpu/pkg/mesh/build"
)

func TestIndependentGeometryStageRemapsLevelUVsBeforeOptionalBake(t *testing.T) {
	t.Parallel()

	for _, production := range []bool{false, true} {
		t.Run(strconv.FormatBool(production), func(t *testing.T) {
			t.Parallel()

			var original assetbuild.Ref

			config := fixtureConfig(syntheticBake())
			config.Pipelines[0].Process = append(config.Pipelines[0].Process[:2],
				LightmapUVs(UVOptions{Production: production, TexelsPerUnit: 4, MaxPageSize: 32, Padding: 2, TransportSamples: 16}),
				geometrySnapshot(&original), meshbuild.Optimize(), geometryVerify(&original))
			config.Pipelines[0].Output = nil
			options := assetbuild.Options{Project: writeFixture(t), Identity: "independent-level-geometry"}

			first, err := assetbuild.Build(t.Context(), config, options)
			if err != nil || first.Written != 3 {
				t.Fatal(first, err)
			}

			warm, err := assetbuild.Build(t.Context(), config, options)
			if err != nil || warm.Executed != 0 || warm.Reused != first.Executed {
				t.Fatal(warm, err)
			}
		})
	}
}

func geometrySnapshot(original *assetbuild.Ref) assetbuild.Plugin {
	return stage{
		name:   "geometry-snapshot",
		schema: uvSchema,
		run: func(_ *assetbuild.Plan, resource assetbuild.Resource) error {
			*original = resource.Ref

			return nil
		},
	}
}

func geometryVerify(original *assetbuild.Ref) assetbuild.Plugin {
	return stage{name: "geometry-verify", schema: uvSchema, run: func(plan *assetbuild.Plan, resource assetbuild.Resource) error {
		return plan.Expand(
			assetbuild.Expansion{
				ID:     "geometry-verify/pages",
				Inputs: []assetbuild.Ref{*original, resource.Ref},
				Run: func(ctx *assetbuild.Context, child *assetbuild.Plan) error {
					var before, after partitioned
					if err := readRecord(ctx, *original, &before); err != nil {
						return err
					}

					if err := readRecord(ctx, resource.Ref, &after); err != nil {
						return err
					}

					var artifacts []assetbuild.Artifact

					for _, record := range []partitioned{before, after} {
						for _, chunk := range record.Chunks {
							for _, page := range chunk.Pages {
								artifacts = append(artifacts, geometryPageArtifacts(page)...)
							}
						}
					}

					refs, err := child.Add(
						assetbuild.Action{
							ID:        "geometry-verify/check",
							Processor: "geometry-verify",
							Revision:  "1",
							Inputs:    []assetbuild.Ref{*original, resource.Ref},
							Artifacts: artifacts,
							Outputs:   []assetbuild.Slot{{Name: filesSlot, Schema: filesSchema}},
							Run: func(ctx *assetbuild.Context) (assetbuild.Result, error) {
								if err := verifyGeometryPages(ctx, before, after); err != nil {
									return nil, err
								}

								data, err := assetbuild.EncodeBundle(
									assetbuild.Bundle{"checked.txt": []byte("geometry and UV references verified")},
								)

								return assetbuild.Result{filesSlot: data}, err
							},
						},
					)
					if err == nil {
						child.Publish(refs[0])
					}

					return err
				},
			},
		)
	}}
}

func verifyGeometryPages(ctx *assetbuild.Context, before, after partitioned) error {
	if len(before.Chunks) != len(after.Chunks) {
		return ErrInvalid
	}

	for index, chunk := range before.Chunks {
		other := after.Chunks[index]
		if len(chunk.Pages) != len(other.Pages) || chunk.Occluders != other.Occluders {
			return ErrInvalid
		}

		for pageIndex, page := range chunk.Pages {
			if err := verifyGeometryPage(ctx, page, other.Pages[pageIndex]); err != nil {
				return err
			}
		}
	}

	return nil
}

func verifyGeometryPage(ctx *assetbuild.Context, before, after pageRecord) error {
	oldModel, oldUV, oldLegacy, err := readGeometryFixture(ctx, before)
	if err != nil {
		return err
	}

	newModel, newUV, newLegacy, err := readGeometryFixture(ctx, after)
	if err != nil {
		return err
	}

	if before.Width != after.Width || before.Height != after.Height || before.GeometricNormals != after.GeometricNormals {
		return ErrInvalid
	}

	if oldLegacy != nil {
		oldLegacy.UVs = nil
		newLegacy.UVs = nil
		oldLegacy.ModelSHA256 = [32]byte{}

		newLegacy.ModelSHA256 = [32]byte{}
		if !reflect.DeepEqual(oldLegacy, newLegacy) {
			return fmt.Errorf("rebuilt legacy bake data: %w", ErrInvalid)
		}
	}

	if before.Receivers.Size == 0 {
		return nil
	}

	oldData, err := ctx.ReadArtifact(before.Receivers)
	if err != nil {
		return err
	}

	newData, err := ctx.ReadArtifact(after.Receivers)
	if err != nil {
		return err
	}

	oldSet, err := bake.DecodeReceiverSet(oldData)
	if err != nil {
		return err
	}

	newSet, err := bake.DecodeReceiverSet(newData)
	if err != nil {
		return err
	}

	if len(oldSet.Receivers) != len(newSet.Receivers) {
		return ErrInvalid
	}

	for index, receiver := range oldSet.Receivers {
		other := &newSet.Receivers[index]
		if other.ID != receiverIdentityFixture(after.Geometry.SHA256, other.Pixel, other.Side) {
			return fmt.Errorf("stale receiver identity: %w", ErrInvalid)
		}

		if len(receiver.Pieces) != len(other.Pieces) {
			return ErrInvalid
		}

		for piece, oldPiece := range receiver.Pieces {
			if geometryCornerSignature(
				oldModel,
				oldUV,
				oldPiece.Triangle,
			) != geometryCornerSignature(
				newModel,
				newUV,
				other.Pieces[piece].Triangle,
			) {
				return fmt.Errorf("stale receiver triangle: %w", ErrInvalid)
			}

			other.Pieces[piece].Triangle = oldPiece.Triangle
		}

		other.ID = receiver.ID
	}

	if !reflect.DeepEqual(oldSet, newSet) {
		return fmt.Errorf("reclipped receiver footprints: %w", ErrInvalid)
	}

	return nil
}

//nolint:gosec // Receiver side retains its signed int32 wire representation.
func receiverIdentityFixture(digest string, pixel uint32, side int32) uint64 {
	model, err := hex.DecodeString(digest)
	if err != nil {
		return 0
	}

	identity := binary.LittleEndian.AppendUint32(model, pixel)
	identity = binary.LittleEndian.AppendUint32(identity, uint32(side))
	sum := sha256.Sum256(identity)

	return binary.LittleEndian.Uint64(sum[:8])
}

func readGeometryFixture(ctx *assetbuild.Context, page pageRecord) (modeldata.Document, [][]byte, *lightmapdata.Document, error) {
	data, err := ctx.ReadArtifact(page.Geometry)
	if err != nil {
		return modeldata.Document{}, nil, nil, err
	}

	model, err := modeldata.Decode(data)
	if err != nil {
		return modeldata.Document{}, nil, nil, err
	}

	uvs, legacy, err := pageGeometryUVs(ctx, page, sha256.Sum256(data))
	if err != nil {
		return modeldata.Document{}, nil, nil, err
	}

	for index, part := range model.Parts {
		if len(uvs[index]) != len(part.Vertices)/4 {
			return modeldata.Document{}, nil, nil, ErrInvalid
		}
	}

	return model, uvs, legacy, nil
}

//nolint:gosec // The admitted test model's index count fits uint32.
func geometryCornerSignature(model modeldata.Document, uvs [][]byte, triangle uint32) string {
	for index, part := range model.Parts {
		count := uint32(len(part.Indices) / 12)
		if triangle >= count {
			triangle -= count

			continue
		}

		var signature []byte

		for corner := range 3 {
			vertex := int(binary.LittleEndian.Uint32(part.Indices[int(triangle)*12+corner*4:]))
			signature = append(signature, part.Vertices[vertex*32:(vertex+1)*32]...)
			signature = append(signature, uvs[index][vertex*8:(vertex+1)*8]...)
		}

		return string(signature)
	}

	return ""
}

func TestGeometryStageRejectsCleanupAfterUVsAndOrderingAfterBake(t *testing.T) {
	t.Parallel()

	for _, late := range []assetbuild.Plugin{meshbuild.Cleanup(), meshbuild.Optimize()} {
		config := fixtureConfig(syntheticBake())
		if late.Name() == "mesh-cleanup" {
			config.Pipelines[0].Process = append(config.Pipelines[0].Process[:3], late)
		} else {
			config.Pipelines[0].Process = append(config.Pipelines[0].Process[:4], late)
		}

		config.Pipelines[0].Output = nil

		if _, err := assetbuild.Build(
			t.Context(),
			config,
			assetbuild.Options{Project: writeFixture(t), Identity: "invalid-geometry-order"},
		); err == nil {
			t.Fatal("accepted invalid stage ordering")
		}
	}
}
