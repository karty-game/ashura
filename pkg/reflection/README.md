# Reflection probes

`reflection.NewProbe` attaches a prepared environment map to a world-space box.
Reflective Standard materials inside the box select the smallest containing
probe automatically. The renderer box-projects each reflected ray from the
surface toward the probe's cubemap, which keeps nearby room features aligned
with the box instead of reflecting as if they were infinitely far away.

```go
environment, err := assets.RoomReflection().Load(app)
if err != nil {
	return err
}

probe := reflection.DefaultProbe("room", environment)
probe.HalfExtents = engine.Vector3{X: 6, Y: 3, Z: 6}
world.Add(
	probe,
	engine.Transform{Position: engine.Vector3{Y: 3}, Scale: engine.Vector3One()},
)

floor := material.DefaultStandard()
floor.Roughness, floor.ReflectionStrength = .15, .8
world.Add(mesh.NewPlane("polished floor", mesh.PlaneOptions{Width: 12, Height: 12}),
	engine.DefaultTransform(), floor)
```

The probe uses the entity's `Transform.Position`; its `HalfExtents` are positive
world-unit distances from that point. Probe `Strength` scales the material's
`ReflectionStrength`. `reflection.DefaultProbe(name, environment)` returns a
probe at unit strength; assign its fields directly, including `Strength = 0`
to disable the probe before attachment. The compatibility `NewProbe` options
constructor treats zero strength as the default of one. Probe
boxes are axis-aligned. Scale and rotation on the probe entity do not change its
influence bounds. Outside all probe boxes, the material has no environment
reflection.

## Baking the environment map

Reflection assets are built offline from a 2:1 equirectangular image. For an
authored PNG/JPEG, name it with a `-reflection` suffix, for example
`assets/room-reflection.png`, and use this pipeline:

```go
Pipelines: []assetbuild.Pipeline{{
	Name: "reflections", Input: []string{"assets/**"},
	Import: []assetbuild.Plugin{assetbuild.Images()},
	Process: []assetbuild.Plugin{assetbuild.ReflectionProbes()},
	Output: []assetbuild.Plugin{assetbuild.KTX2(), assetbuild.GoCatalog()},
}},
```

`ReflectionProbes()` converts the panorama to a six-face horizontal atlas in
`+X, -X, +Y, -Y, +Z, -Z` order and filters each face's mip chain separately.
Generated catalogs expose a `texture/ktx2.Reflection` asset. Source decoding,
projection and mip filtering happen during the build; runtime only uploads the
prepared KTX2 texture and samples it. The plugin bakes an environment panorama,
not a live scene capture or arbitrary 3D scene.

For a primitive scene, the shared [reflection build module](build/README.md)
imports chunk JSON and bakes a bounded analytic diffuse panorama. It generates
matching typed scene declarations from the same source. Its 32 m chunk/neighborhood
policy preserves separate spaces and incremental reuse; the sample owns only
its authored data and recipe. No sample-specific generator is required.

The current Standard shader adds a roughness-selected mip reflection with a
Schlick Fresnel weight. It remains a lightweight diffuse/specular material, not
a full metallic/roughness PBR model. Reflection probes are available on native
and browser renderers through the same WGSL implementation.

See the [room sample](../../samples/reflections/) for a complete panorama recipe,
probe placement and reflective floor, walls and ceiling.
