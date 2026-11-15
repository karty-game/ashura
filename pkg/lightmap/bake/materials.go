//go:build !js

package lightmapbake

import (
	"strings"

	"github.com/karty-game/karty-webgpu/internal/modeldata"
)

// RemapReceiverMaterials binds independently triangulated receiver support to
// the complete visibility scene's stable material identities. Back-side receiver
// geometry shares its original surface material while receiving its own map.
func RemapReceiverMaterials(model []byte, scene *PreparedScene, receivers []Receiver) error {
	if scene == nil {
		return ErrInvalid
	}

	doc, err := modeldata.Decode(model)
	if err != nil {
		return err
	}

	materials := make(map[string]uint32)
	bindings := make(map[string]uint32)

	for index, material := range scene.Source().Materials {
		if material.ID != "" {
			materials[material.ID] = uint32(index)
			if binding, _, found := strings.Cut(material.ID, "/"); found && strings.HasPrefix(binding, "material:") {
				bindings[binding] = uint32(index)
			}
		}
	}

	remap := make([]uint32, len(doc.Parts))
	for index, part := range doc.Parts {
		target, ok := bindings[reflectanceBinding(doc, part.Material)]
		if !ok {
			target, ok = materials[strings.TrimSuffix(part.Name, "@back")]
		}

		if !ok {
			return ErrInvalid
		}

		remap[index] = target
	}

	for i := range receivers {
		for j := range receivers[i].Patches {
			patch := &receivers[i].Patches[j]
			if uint64(patch.Material) >= uint64(len(remap)) {
				return ErrInvalid
			}

			patch.Material = remap[patch.Material]
		}
	}

	return nil
}
