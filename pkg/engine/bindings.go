package engine

// Providers are resolved when component structure changes. Ordinary frames read
// typed pointers/interfaces and compare values; they never reflect or traverse
// every component of every renderable to rediscover its material and geometry.
type entityBindings struct {
	owner            meshOwnerComponent
	free             freeControlsComponent
	orbit            orbitControlsComponent
	pan              panZoomControlsComponent
	height           viewHeightComponent
	transform        *Transform
	mesh             meshComponent
	prepared         preparedMeshComponent
	identity         meshIdentity
	sourceBatch      *sourceBatches
	billboard        billboardComponent
	material         MaterialProvider
	alphaMask        alphaMaskMaterialComponent
	culled           culledMaterialComponent
	source           sourceMaterialComponent
	reflection       reflectionMaterialComponent
	lightmap         lightmapMaterialComponent
	lightmapEncoding encodedLightmapMaterialComponent
	rnm              rnmLightmapMaterialComponent
	probe            reflectionProbeComponent
	impostor         impostorMaterialComponent
	impostorQuality  impostorQualityComponent
	textured         texturedMaterialComponent
	occlusion        occlusionMaterialComponent
	projection       projectionBinding
	lights           []lightBinding
	decalProjector   *DecalProjector
	decalReceiver    *DecalReceiver
}

func (world *World) refreshBindings(entity Entity) {
	if world.bindings == nil {
		world.bindings = make(map[Entity]*entityBindings)
	}

	entry := world.bindings[entity]
	if entry == nil {
		entry = &entityBindings{}
		world.bindings[entity] = entry
	}

	lights := entry.lights[:0]
	previousDecal := entry.decalProjector
	*entry = entityBindings{lights: lights}

	world.eachComponent(entity, func(component any) {
		if value, ok := component.(meshOwnerComponent); ok {
			entry.owner = value
		}

		entry.bindNavigation(component)

		entry.bindDecals(component)

		if value, ok := component.(*Transform); ok {
			entry.transform = value
		}

		if value, ok := component.(meshComponent); ok {
			entry.mesh = value
		}

		if value, ok := component.(preparedMeshComponent); ok {
			entry.prepared = value
		}

		if value, ok := component.(meshIdentity); ok {
			entry.identity = value
		}

		if value, ok := component.(billboardComponent); ok {
			entry.billboard = value
		}

		entry.bindMaterial(component)

		if value, ok := component.(reflectionProbeComponent); ok {
			entry.probe = value
		}

		if value, ok := component.(ProjectionProvider); ok {
			entry.projection = projectionBinding{typed: value}
		} else if value, ok := component.(cameraComponent); ok {
			entry.projection = projectionBinding{legacy: value}
		}

		if value, ok := component.(LightProvider); ok {
			entry.lights = append(entry.lights, lightBinding{typed: value})
		} else if value, ok := component.(lightComponent); ok {
			shadow, _ := component.(pointShadowComponent)
			debug, _ := component.(pointShadowDebugComponent)
			entry.lights = append(entry.lights, lightBinding{legacy: value, shadow: shadow, debug: debug})
		}
	})

	if previousDecal != entry.decalProjector {
		world.bindDecal(entity, entry.decalProjector)
	}

	world.updateCameraBinding(entity, entry)
	world.frame.invalidate(entity.index, entry.mesh != nil || entry.prepared != nil || entry.owner != nil)
}

func (world *World) updateCameraBinding(entity Entity, entry *entityBindings) {
	eligible := entry != nil && entry.projection.valid() && entry.transform != nil
	if !eligible && world.active == entity {
		world.active = Entity{}
	}

	for index, current := range world.cameras {
		if current == entity {
			if !eligible {
				copy(world.cameras[index:], world.cameras[index+1:])
				world.cameras[len(world.cameras)-1] = Entity{}
				world.cameras = world.cameras[:len(world.cameras)-1]
			}

			return
		}

		if eligible && current.index > entity.index {
			world.cameras = append(world.cameras, Entity{})
			copy(world.cameras[index+1:], world.cameras[index:])
			world.cameras[index] = entity

			return
		}
	}

	if eligible {
		world.cameras = append(world.cameras, entity)
	}
}

func (entry *entityBindings) bindNavigation(component any) {
	if value, ok := component.(freeControlsComponent); ok {
		entry.free = value
	}

	if value, ok := component.(orbitControlsComponent); ok {
		entry.orbit = value
	}

	if value, ok := component.(panZoomControlsComponent); ok {
		entry.pan = value
	}

	if value, ok := component.(viewHeightComponent); ok {
		entry.height = value
	}
}

func (entry *entityBindings) bindMaterial(component any) {
	if provider, ok := component.(MaterialProvider); ok {
		entry.material = provider
		entry.source, _ = component.(sourceMaterialComponent)
		entry.impostor, _ = component.(impostorMaterialComponent)
		entry.impostorQuality, _ = component.(impostorQualityComponent)
		entry.textured, _ = component.(texturedMaterialComponent)
		entry.occlusion, _ = component.(occlusionMaterialComponent)
		entry.culled, _ = component.(culledMaterialComponent)
		entry.alphaMask, _ = component.(alphaMaskMaterialComponent)
		entry.reflection, _ = component.(reflectionMaterialComponent)
		entry.lightmap, _ = component.(lightmapMaterialComponent)
		entry.lightmapEncoding, _ = component.(encodedLightmapMaterialComponent)
		entry.rnm, _ = component.(rnmLightmapMaterialComponent)

		return
	}

	if entry.material != nil {
		return
	}

	if provider, ok := component.(sourceMaterialComponent); ok {
		entry.source = provider
	}

	if provider, ok := component.(impostorMaterialComponent); ok {
		entry.impostor = provider
		entry.impostorQuality, _ = component.(impostorQualityComponent)
	}

	if provider, ok := component.(texturedMaterialComponent); ok {
		entry.textured = provider
	}

	if provider, ok := component.(occlusionMaterialComponent); ok {
		entry.occlusion = provider
	}

	if provider, ok := component.(alphaMaskMaterialComponent); ok {
		entry.alphaMask = provider
	}

	if provider, ok := component.(culledMaterialComponent); ok {
		entry.culled = provider
	}

	if provider, ok := component.(rnmLightmapMaterialComponent); ok {
		entry.rnm = provider
	}

	if provider, ok := component.(reflectionMaterialComponent); ok {
		entry.reflection = provider
	}
}
