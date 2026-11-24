package engine

import (
	"github.com/karty-game/ashura/internal/impostordata"
	"github.com/karty-game/ashura/internal/meshdata"
	"github.com/karty-game/ashura/internal/render"
	"github.com/karty-game/ashura/pkg/texture"
)

type cameraComponent interface {
	RuntimeProjection() (kind uint8, fieldOfView, viewHeight, near, far float32)
}

type lightComponent interface {
	RuntimeLight() (kind uint32, direction, color, groundColor [3]float32, intensity float32)
}

type pointShadowDebugComponent interface {
	RuntimePointShadowDebug() bool
}

type pointShadowComponent interface {
	RuntimePointShadow() (cast bool, resolution uint32, bias float32)
}

type meshComponent interface {
	RuntimeMesh() (positions []Vector3, indices []uint32, normals []Vector3, uvs []Vector2, billboard bool)
}

type meshOwnerComponent interface {
	RuntimeMeshOwner() (*App, bool)
}

type preparedMeshComponent interface {
	RuntimePreparedMesh() *meshdata.Geometry
}

type billboardComponent interface {
	RuntimeBillboard() (uint8, Vector3)
}

type meshIdentity interface {
	RuntimeMeshIdentity() any
}

// MaterialProvider supplies the tint and roughness of a material component.
// Standard and Source materials implement it, with additional methods for their maps.
type MaterialProvider interface {
	RuntimeMaterial() (color Color, roughness float32)
}

type alphaMaskMaterialComponent interface {
	RuntimeAlphaMask() (enabled bool, cutoff float32)
}

type culledMaterialComponent interface {
	RuntimeCullBackfaces() bool
}

type sourceMaterialComponent interface {
	RuntimeSource() (ambient [3]Color, halfLambert bool, strength float32)
}

type reflectionMaterialComponent interface {
	RuntimeReflectionStrength() float32
}

type lightmapMaterialComponent interface {
	RuntimeLightmap() (*texture.Texture, float32, bool)
}

type encodedLightmapMaterialComponent interface {
	RuntimeLightmapEncoding() (texture.RNMEncoding, bool)
}

type rnmLightmapMaterialComponent interface {
	RuntimeRNMLightmap() *texture.RNMLightmap
}

type reflectionProbeComponent interface {
	RuntimeReflectionProbe() (environment *texture.Texture, halfExtents Vector3, strength float32)
}

type impostorQualityComponent interface {
	RuntimeImpostorQuality() [4]float32
}

type impostorMaterialComponent interface {
	RuntimeImpostor() *impostordata.Document
}

type projectionSettings struct {
	kind               uint8
	fieldOfViewDegrees float32
	viewHeight         float32
	near               float32
	far                float32
}

func projectionForEntity(world *World, entity Entity) (projectionSettings, bool) {
	if world == nil {
		return projectionSettings{}, false
	}

	entry := world.bindings[entity]
	if entry == nil || !entry.projection.valid() {
		return projectionSettings{}, false
	}

	data := entry.projection.read()

	return projectionSettings{
		kind: uint8(data.Kind), fieldOfViewDegrees: data.FieldOfViewDegrees,
		viewHeight: data.ViewHeight, near: data.Near, far: data.Far,
	}, true
}

func projectionIsPerspective(projection projectionSettings) bool {
	return projection.kind == render.PerspectiveProjection
}
