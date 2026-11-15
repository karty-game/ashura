package lightmapper

import "slices"

// WithLighting reuses an admitted scene's actual geometry BVH, materials and
// texture storage. Only emitter preparation, its hierarchy and environment
// proposal are rebuilt. Changing geometry/materials requires PrepareScene.
func WithLighting(base *PreparedScene, emitters []Emitter, environment Environment) (*PreparedScene, error) {
	if base == nil || len(emitters) > 1<<20 || !positiveVec(environment.Sky) || !positiveVec(environment.Ground) ||
		uint64(environment.Texture) > uint64(len(base.source.Textures)) {
		return nil, ErrInvalid
	}

	source := base.source
	source.Emitters = slices.Clone(emitters)
	source.Environment = environment

	prepared := &PreparedScene{
		source:             source,
		nodes:              base.nodes,
		order:              base.order,
		geometryHash:       base.geometryHash,
		triangleEmitters:   make([]int32, len(source.Triangles)),
		environmentEmitter: -1,
	}
	for i := range prepared.triangleEmitters {
		prepared.triangleEmitters[i] = -1
	}

	if err := prepared.prepareEmitters(); err != nil {
		return nil, err
	}

	if err := prepared.rebuildLighting(); err != nil {
		return nil, err
	}

	if err := prepared.validateSurfaceOwnership(); err != nil {
		return nil, err
	}

	if err := prepared.finishSceneIdentity(); err != nil {
		return nil, err
	}

	return prepared, nil
}

func (prepared *PreparedScene) rebuildLighting() error {
	prepared.buildLights()
	prepared.buildEnvironment()

	for _, n := range prepared.lightNodes {
		if !finite(n.Power) || !globalPowerFinite(n) || !validVec(n.Min) || !validVec(n.Max) || !finite(n.ConeCos) ||
			!validVec(n.Direction) {
			return ErrBounds
		}
	}

	return nil
}
